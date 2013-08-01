package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/FreekKalter/ansi/color"
	"github.com/FreekKalter/text/columnswriter"
	"github.com/FreekKalter/text/tabwriter"
)

type colorFunc func(interface{}) *color.Escape

var colorMap map[string]colorFunc = map[string]colorFunc{
	"ok":                 func(i interface{}) *color.Escape { return color.BgDefault(color.Default(i)) },
	"file":               func(i interface{}) *color.Escape { return color.BgDefault(color.Default(i)) },
	"no_version_control": func(i interface{}) *color.Escape { return color.Bold(color.Blue(i)) },                  // Blue
	"dirty":              func(i interface{}) *color.Escape { return color.Bold(color.Red(i)) },                   // Red
	"no_remote":          func(i interface{}) *color.Escape { return color.BgBlue(color.Bold(color.Red(i))) },     // Red on Blue
	"fetch_failed":       func(i interface{}) *color.Escape { return color.BgRed(color.Bold(color.Blue(i))) },     // Blue on Red
	"branch_ahead":       func(i interface{}) *color.Escape { return color.BgYellow(color.Bold(color.Green(i))) }, // Green on Yellow
	"branch_behind":      func(i interface{}) *color.Escape { return color.BgYellow(color.Bold(color.Red(i))) },   // Red on Yellow
}

// Struct passed between gls and main
type Project struct {
	Name, State string
	Info        os.FileInfo
}
type Projects []*Project

func (projects Projects) Len() int      { return len(projects) }
func (projects Projects) Swap(i, j int) { projects[i], projects[j] = projects[j], projects[i] }

type ByName struct{ Projects }
type ByState struct{ Projects }

func (s ByName) Less(i, j int) bool {
	return strings.ToLower(s.Projects[i].Name) < strings.ToLower(s.Projects[j].Name)
}
func (s ByState) Less(i, j int) bool {
	return sortOrderStates[s.Projects[i].State] < sortOrderStates[s.Projects[j].State]
}

var (
	cleanGitRegex = regexp.MustCompile("nothing to commit")
	fetchErrors   = regexp.MustCompile("^fatal")
	branchAhead   = regexp.MustCompile("branch is ahead of")
	branchBehind  = regexp.MustCompile("branch is behind")
)
var help, list, onlyDirty, sortByState, all bool
var sortOrderStates = map[string]int{"ok": 0, "no_version_control": 1, "dirty": 2, "no_remote": 3, "fetch_failed": 4, "branch_ahead": 5, "branch_behind": 6}
var TimeFormat = "Jan 2,2006 15:04"
var wg sync.WaitGroup

func main() {
	flag.BoolVar(&help, "help", false, "print help message")
	flag.BoolVar(&list, "list", false, "display results in 1 long list")
	flag.BoolVar(&all, "all", false, "display files and folders staring with a dot")
	flag.BoolVar(&onlyDirty, "dirty", false, "only show diry dirs, this is very fast because it does not check remotes")
	flag.BoolVar(&sortByState, "statesort", false, "sort output by state")
	flag.Parse()
	if help {
		flag.Usage()
		fmt.Println("")
		fmt.Println("Color codes:")
		for k, v := range colorMap {
			fmt.Println(v(k))
		}
		return
	}

    // Sort out path and files in that dir
	var path string
	if len(flag.Args()) > 0 {
		path = flag.Arg(0)
	} else {
		path = "."
	}
	var files []string
	var err error
	if all {
		files, err = filepath.Glob(filepath.Join(path, "*"))
	} else {
		files, err = filepath.Glob(filepath.Join(path, "[^.]*"))
	}
	if err != nil {
		panic(err)
	}

    // Start goroutine for every dir found
	glsResults := make(chan *Project, 1000)
	var projects Projects
	for _, file := range files {
		file_info, _ := os.Stat(file)
		if file_info.IsDir() {
			wg.Add(1)
			go gls(&Project{Name: file, Info: file_info}, glsResults)
		} else {
			projects = append(projects, &Project{Name: file, State: "file", Info: file_info})
		}
	}
	wg.Wait()
	close(glsResults)


    // Gather results and process them
	for res := range glsResults {
		// make a copy to add to []projects, because res always points to the same address space
		toAppend := res
		toAppend.Name = filepath.Base(res.Name)
		projects = append(projects, toAppend)
	}
	if sortByState {
		sort.Sort(ByState{projects})
	} else {
		sort.Sort(ByName{projects})
	}

	if list {
		//TODO: 0 pad dates
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 0, 8, 0, '\t', tabwriter.StripEscape)
		for _, p := range projects {
			fmt.Fprintf(w, "%s\t%s\t%s\n", colorMap[p.State](p.Name), humanReadable(p.Info.Size()), p.Info.ModTime().Format(TimeFormat))
		}
		w.Flush()
	} else {

		var projectsString string
		for _, p := range projects {
			projectsString = fmt.Sprintf("%s\t%s", projectsString, colorMap[p.State](p.Name))
		}

		w := columnswriter.New(os.Stdout, '\t', 0, 2)
		fmt.Fprint(w, projectsString)
		w.Flush()
	}
}

func gls(project *Project /*dirName string*/, result chan *Project) {
	defer wg.Done()

	// First chek, is the directory under (git) version control
	if ok, _ := exists(filepath.Join(project.Name, ".git")); !ok {
		if !onlyDirty {
			project.State = "no_version_control"
			result <- project
		}
		return
	}

	gitDir := fmt.Sprintf("--git-dir=%s", filepath.Join(project.Name, ".git"))
	gitTree := fmt.Sprintf("--work-tree=%s", project.Name)
	output, err := exec.Command("git", gitDir, gitTree, "status").Output() //, gitDir, gitTree, "status")
	if err != nil {
		panic(err)
	}
	// Are there uncommitted changes is the directory (dirty)
	if !cleanGitRegex.MatchString(strings.TrimSpace(string(output))) {
		project.State = "dirty"
		result <- project
		return
	} else if onlyDirty {
		return
	}

	// Check if the repo has a remote
	output, err = exec.Command("git", gitDir, gitTree, "remote", "-v").Output()
	if err != nil {
		panic(err)
	}
	if len(output) == 0 {
		project.State = "no_remote"
		result <- project
		return
	}

	// Fetch latest changes from remote
	output, err = exec.Command("git", gitDir, gitTree, "fetch").Output()
	if err != nil {
		project.State = "fetch_failed"
		result <- project
		return
	}
	outputStr := strings.TrimSpace(string(output))
	if fetchErrors.MatchString(outputStr) {
		project.State = "fetch_failed"
		result <- project
		return
	}

	output, err = exec.Command("git", gitDir, gitTree, "status").Output()
	if err != nil {
		panic(err)
	}
	outputStr = strings.TrimSpace(string(output))

	// Is branch ahead of behind of remote
	if branchAhead.MatchString(outputStr) {
		project.State = "branch_ahead"
		result <- project
		return
	} else if branchBehind.MatchString(outputStr) {
		project.State = "branch_behind"
		result <- project
		return
	}

	project.State = "ok"
	result <- project
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func humanReadable(filesize int64) string {
	fs := float64(filesize)
	for _, x := range []string{"b", "kb", "mb", "gb", "tb"} {
		if fs < 1024 {
			return fmt.Sprintf("%3.1f % s", fs, x)
		}
		fs /= 1024
	}
	return ""
}
