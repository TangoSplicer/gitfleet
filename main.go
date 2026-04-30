package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

type mainModel struct {
	state      AppState
	fleet      *Fleet
	cfg        Config
	rootPath   string
	totalRepos int
	reposDone  int
	results    []Result
}

func initialModel(cfg Config, targetDir string) mainModel {
	return mainModel{
		state:    StateDashboard,
		cfg:      cfg,
		rootPath: targetDir,
		results:  make([]Result, 0),
	}
}

func (m mainModel) Init() tea.Cmd { return nil }

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		m.results = append(m.results, Result(msg))
		if m.reposDone >= m.totalRepos {
			return m, func() tea.Msg { return SwarmDoneMsg{} }
		}
		return m, waitForResult(m.fleet.Results)

	case SwarmDoneMsg:
		m.state = StateReport
		return m, nil
	}
	return m, nil
}

func (m mainModel) View() string {
	switch m.state {
	case StateDashboard:
		return fmt.Sprintf("GitFleet Dashboard\nTarget: %s\nWorkers: %d\nPress 'Enter' to start Morning Routine, 'q' to quit.", m.rootPath, m.fleetWorkerCount())
	case StateSwarming:
		return fmt.Sprintf("Swarming with %d workers...\nProgress: %d/%d repos processed.", m.fleetWorkerCount(), m.reposDone, m.totalRepos)
	case StateReport:
		successes := 0
		for _, r := range m.results {
			if r.Success {
				successes++
			}
		}
		return fmt.Sprintf("Report for %s:\nTotal: %d | Success: %d | Failed: %d\nPress 'q' to quit.",
			truncatePath(m.rootPath, 40), m.totalRepos, successes, m.totalRepos-successes)
	default:
		return "Unknown state"
	}
}

// Helper to determine active workers for the UI
func (m *mainModel) fleetWorkerCount() int {
	if m.fleet != nil {
		return m.fleet.NumWorkers
	}
	return getOptimalWorkers(m.cfg.MaxWorkers)
}

func (m *mainModel) startSwarm(swarmType SwarmType) (tea.Model, tea.Cmd) {
	m.state = StateSwarming
	
	// Adaptive Worker Allocation
	workers := getOptimalWorkers(m.cfg.MaxWorkers)
	m.fleet = NewFleet(workers)
	m.reposDone = 0
	m.results = nil

	scanCmd := func() tea.Msg {
		repoCount := 0
		filepath.WalkDir(m.rootPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil { return nil }

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
		if !ok { return nil }
		return ResultMsg(res)
	}
}

func truncatePath(path string, maxLength int) string {
	if len(path) <= maxLength {
		return path
	}
	parts := strings.Split(path, string(os.PathSeparator))
	if len(parts) < 3 {
		return "..." + path[len(path)-maxLength+3:]
	}
	return ".../" + filepath.Join(parts[len(parts)-2:]...)
}

// getOptimalWorkers calculates pool size based on hardware limits
func getOptimalWorkers(configOverride int) int {
	if configOverride > 0 {
		return configOverride // User forced a specific number
	}
	
	// Git ops are I/O bound, not CPU bound. We can safely multiply cores by 2.
	cpus := runtime.NumCPU()
	workers := cpus * 2
	
	// Guardrails: Don't choke Termux (min 4), don't spam desktop OS (max 32)
	if workers < 4 { return 4 }
	if workers > 32 { return 32 }
	
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
