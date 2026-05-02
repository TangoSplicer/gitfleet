package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AppState int

const (
	StateDashboard AppState = iota
	StateSwarming
	StateReport
)

type ResultMsg Result
type SwarmDoneMsg struct{}
type ScanCompleteMsg struct{ TotalRepos int }
type RefreshCountMsg struct{ TotalRepos int }
type tickMsg time.Time

type item struct {
	title, desc string
	swarmType   SwarmType
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type mainModel struct {
	state        AppState
	fleet        *Fleet
	cfg          Config
	rootPath     string
	totalRepos   int
	reposDone    int
	results      []Result
	windowWidth  int
	windowHeight int

	menu     list.Model
	progress progress.Model
	logs     []string
}

func initialModel(cfg Config, targetDir string) mainModel {
	items := []list.Item{
		item{title: "🌅 Morning Routine", desc: "Check git status of all local repos", swarmType: SwarmStatus},
		item{title: "🔄 Sync Swarm", desc: "Perform concurrent git pull --rebase", swarmType: SwarmSync},
		item{title: "🧹 Cleanup Crew", desc: "Prune dead remote tracking branches", swarmType: SwarmPrune},
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(ColorPrimary).BorderLeftForeground(ColorPrimary)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(ColorSecondary).BorderLeftForeground(ColorPrimary)

	m := list.New(items, delegate, 0, 0)
	m.SetShowTitle(false)
	m.SetShowStatusBar(false)
	m.SetFilteringEnabled(false)

	return mainModel{
		state:    StateDashboard,
		cfg:      cfg,
		rootPath: targetDir,
		results:  make([]Result, 0),
		menu:     m,
		progress: progress.New(progress.WithDefaultGradient()),
		logs:     make([]string, 0),
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(m.refreshScanCmd(), doTick())
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height

		// Strict internal bounds calculation
		boxWidth := msg.Width - 2
		boxHeight := msg.Height - 2

		// Calculate precise remaining space for the menu
		// Box padding/borders takes ~4 lines
		// Header takes ~4 lines
		listHeight := boxHeight - 8
		if listHeight < 5 {
			listHeight = 5 // Failsafe for extreme minimum height
		}

		m.menu.SetSize(boxWidth-4, listHeight)

		m.progress.Width = msg.Width - 10
		if m.progress.Width > 60 {
			m.progress.Width = 60
		}
		return m, nil

	case tickMsg:
		if m.state == StateDashboard {
			return m, tea.Batch(m.refreshScanCmd(), doTick())
		}
		return m, doTick()

	case RefreshCountMsg:
		m.totalRepos = msg.TotalRepos
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.fleet != nil {
				m.fleet.Stop()
			}
			return m, tea.Quit
		case "enter":
			if m.state == StateDashboard {
				selected := m.menu.SelectedItem().(item)
				return m.startSwarm(selected.swarmType)
			}
		}

	case ScanCompleteMsg:
		m.totalRepos = msg.TotalRepos
		if m.totalRepos == 0 {
			m.state = StateReport
			return m, nil
		}
		m.fleet.Start()
		return m, waitForResult(m.fleet.Results)

	case ResultMsg:
		m.reposDone++
		res := Result(msg)
		m.results = append(m.results, res)

		statusTxt := StyleSuccess.Render("OK")
		if !res.Success {
			statusTxt = StyleError.Render("FAIL")
		}

		logLine := fmt.Sprintf("[%s] %s", statusTxt, truncatePath(res.Path, 25))
		m.logs = append(m.logs, logLine)
		if len(m.logs) > 3 {
			m.logs = m.logs[1:]
		}

		percent := float64(m.reposDone) / float64(m.totalRepos)
		cmd := m.progress.SetPercent(percent)

		if m.reposDone >= m.totalRepos {
			return m, tea.Batch(cmd, func() tea.Msg { return SwarmDoneMsg{} })
		}
		return m, tea.Batch(cmd, waitForResult(m.fleet.Results))

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case SwarmDoneMsg:
		m.state = StateReport
		return m, nil
	}

	if m.state == StateDashboard {
		var cmd tea.Cmd
		m.menu, cmd = m.menu.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m mainModel) View() string {
	// The Absolute Bounding Box with strict Padding/Margin enforcement
	dynamicBox := StyleMainBox.Copy().
		Margin(0).
		Padding(1, 2).
		Width(m.windowWidth - 2).
		Height(m.windowHeight - 2)

	var content string

	switch m.state {
	case StateDashboard:
		headerStr := fmt.Sprintf("%s %s | %s %d",
			StyleSubtle.Render("Target:"), StyleHighlight.Render(truncatePath(m.rootPath, 30)),
			StyleSubtle.Render("Live Repos:"), m.totalRepos,
		)

		// JoinVertical prevents newline expansion bugs
		content = lipgloss.JoinVertical(lipgloss.Left,
			StyleTitle.Render("⛵ GitFleet Workspace"),
			headerStr,
			"", // Add clear spacing
			m.menu.View(),
		)

	case StateSwarming:
		logText := strings.Join(m.logs, "\n")
		content = lipgloss.JoinVertical(lipgloss.Left,
			StyleTitle.Render("🐝 Swarming..."),
			m.progress.View(),
			"",
			StyleLogBox.Render(logText),
		)

	case StateReport:
		successes, failures := 0, 0
		for _, r := range m.results {
			if r.Success {
				successes++
			} else {
				failures++
			}
		}

		stats := fmt.Sprintf("%s %d\n%s %d\n%s %d",
			StyleSubtle.Render("Total scanned:"), m.totalRepos,
			StyleSuccess.Render("Successful:   "), successes,
			StyleError.Render("Failed:       "), failures,
		)
		footer := fmt.Sprintf("Press %s to exit.", StyleHighlight.Render("'q'"))

		content = lipgloss.JoinVertical(lipgloss.Left,
			StyleTitle.Render("📊 Swarm Report"),
			stats,
			"",
			footer,
		)

	default:
		content = "Unknown state"
	}

	// Place strictly centers the box and enforces the exact terminal viewport limits
	return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, dynamicBox.Render(content))
}

func doTick() tea.Cmd {
	return tea.Tick(time.Second*10, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *mainModel) refreshScanCmd() tea.Cmd {
	return func() tea.Msg {
		count := 0
		filepath.WalkDir(m.rootPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				for _, ignore := range m.cfg.IgnoreList {
					if d.Name() == ignore {
						return filepath.SkipDir
					}
				}
			}
			if d.IsDir() && d.Name() == ".git" {
				count++
				return filepath.SkipDir
			}
			return nil
		})
		return RefreshCountMsg{TotalRepos: count}
	}
}

func (m *mainModel) fleetWorkerCount() int {
	if m.fleet != nil {
		return m.fleet.NumWorkers
	}
	return getOptimalWorkers(m.cfg.MaxWorkers)
}

func (m *mainModel) startSwarm(swarmType SwarmType) (tea.Model, tea.Cmd) {
	m.state = StateSwarming
	workers := getOptimalWorkers(m.cfg.MaxWorkers)
	m.fleet = NewFleet(workers)
	m.reposDone = 0
	m.results = nil
	m.logs = []string{"Scanner initializing..."}

	scanCmd := func() tea.Msg {
		repoCount := 0
		filepath.WalkDir(m.rootPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				for _, ignore := range m.cfg.IgnoreList {
					if d.Name() == ignore {
						return filepath.SkipDir
					}
				}
			}
			if d.IsDir() && d.Name() == ".git" {
				repoPath := filepath.Dir(path)
				m.fleet.Jobs <- Job{Path: repoPath, Type: swarmType}
				repoCount++
				return filepath.SkipDir
			}
			return nil
		})
		close(m.fleet.Jobs)
		return ScanCompleteMsg{TotalRepos: repoCount}
	}
	return *m, scanCmd
}

func waitForResult(results <-chan Result) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-results
		if !ok {
			return nil
		}
		return ResultMsg(res)
	}
}

func truncatePath(path string, maxLength int) string {
	if len(path) <= maxLength {
		return path
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 3 {
		return ".../" + filepath.Base(path)
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}

func getOptimalWorkers(configOverride int) int {
	if configOverride > 0 {
		return configOverride
	}
	cpus := runtime.NumCPU()
	workers := cpus * 2
	if workers < 4 {
		return 4
	}
	if workers > 32 {
		return 32
	}
	return workers
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", err)
	}
	var dirFlag string
	flag.StringVar(&dirFlag, "dir", "", "Target workspace directory")
	flag.Parse()
	targetDir := cfg.DefaultWorkspace
	if dirFlag != "" {
		targetDir = dirFlag
	} else if flag.NArg() > 0 {
		targetDir = flag.Arg(0)
	}
	if strings.HasPrefix(targetDir, "~") {
		home, _ := os.UserHomeDir()
		targetDir = filepath.Join(home, targetDir[1:])
	}
	targetDir, err = filepath.Abs(targetDir)
	if err != nil || targetDir == "" {
		fmt.Println("Error resolving path")
		os.Exit(1)
	}
	p := tea.NewProgram(initialModel(cfg, targetDir), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error starting gitfleet: %v\n", err)
		os.Exit(1)
	}
}
