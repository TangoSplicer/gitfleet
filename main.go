package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type AppState int

const (
	StateDashboard AppState = iota
	StateSwarming
	StateReport
)

type ResultMsg Result
type SwarmDoneMsg struct{}
type ScanCompleteMsg struct {
	TotalRepos int
}

// tickMsg is used to animate the progress bar smoothly
type tickMsg struct{}

type mainModel struct {
	state      AppState
	fleet      *Fleet
	cfg        Config
	rootPath   string
	totalRepos int
	reposDone  int
	results    []Result

	// UI Components
	progress progress.Model
	logs     []string // Keeps track of the last few processed repos
}

func initialModel(cfg Config, targetDir string) mainModel {
	return mainModel{
		state:    StateDashboard,
		cfg:      cfg,
		rootPath: targetDir,
		results:  make([]Result, 0),
		progress: progress.New(progress.WithDefaultGradient()),
		logs:     make([]string, 0),
	}
}

func (m mainModel) Init() tea.Cmd { return nil }

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Responsive design: adjust progress bar width based on terminal size
		m.progress.Width = msg.Width - 10
		if m.progress.Width > 60 {
			m.progress.Width = 60 // Cap width on desktop monitors
		}
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
				return m.startSwarm(SwarmStatus)
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

		// Add to UI log queue (keep only last 3 for mobile layout)
		statusTxt := StyleSuccess.Render("OK")
		if !res.Success {
			statusTxt = StyleError.Render("FAIL")
		}
		logLine := fmt.Sprintf("[%s] %s", statusTxt, truncatePath(res.Path, 25))
		m.logs = append(m.logs, logLine)
		if len(m.logs) > 3 {
			m.logs = m.logs[1:] // Pop the oldest log
		}

		// Calculate progress percentage
		percent := float64(m.reposDone) / float64(m.totalRepos)
		cmd := m.progress.SetPercent(percent)

		if m.reposDone >= m.totalRepos {
			return m, tea.Batch(cmd, func() tea.Msg { return SwarmDoneMsg{} })
		}
		return m, tea.Batch(cmd, waitForResult(m.fleet.Results))

	case progress.FrameMsg:
		// Required for progress bar animation
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case SwarmDoneMsg:
		m.state = StateReport
		return m, nil
	}
	return m, nil
}

func (m mainModel) View() string {
	switch m.state {
	case StateDashboard:
		content := fmt.Sprintf("%s\n\nTarget Dir: %s\nWorkers:    %d\n\nPress %s to begin Swarm.",
			StyleTitle.Render("⛵ GitFleet Dashboard"),
			StyleHighlight.Render(truncatePath(m.rootPath, 30)),
			m.fleetWorkerCount(),
			StyleHighlight.Render("'Enter'"),
		)
		return StyleMainBox.Render(content)

	case StateSwarming:
		// Render Logs
		logText := ""
		for _, l := range m.logs {
			logText += l + "\n"
		}

		content := fmt.Sprintf("%s\n\n%s\n\n%s",
			StyleTitle.Render("🐝 Swarming..."),
			m.progress.View(),
			StyleLogBox.Render(logText),
		)
		return StyleMainBox.Render(content)

	case StateReport:
		successes := 0
		failures := 0
		for _, r := range m.results {
			if r.Success {
				successes++
			} else {
				failures++
			}
		}

		content := fmt.Sprintf("%s\n\n%s %d\n%s %d\n%s %d\n\nPress %s to exit.",
			StyleTitle.Render("📊 Swarm Report"),
			StyleSubtle.Render("Total scanned:"), m.totalRepos,
			StyleSuccess.Render("Successful:   "), successes,
			StyleError.Render("Failed:       "), failures,
			StyleHighlight.Render("'q'"),
		)
		return StyleMainBox.Render(content)

	default:
		return "Unknown state"
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
