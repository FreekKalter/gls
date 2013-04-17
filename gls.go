package main

import (
	"flag"
	"fmt"
	"github.com/FreekKalter/text/columnswriter"
	"github.com/str1ngs/ansi/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var wg sync.WaitGroup

type colorFunc func(interface{}) *color.Escape

var colorMap map[string]colorFunc = map[string]colorFunc{
	"no_version_control": func(i interface{}) *color.Escape { return color.Bold(color.Blue(i)) },                  // Blue
	"dirty":              func(i interface{}) *color.Escape { return color.Bold(color.Red(i)) },                   // Red
	"no_remote":          func(i interface{}) *color.Escape { return color.BgBlue(color.Bold(color.Red(i))) },     // Red on Blue
	"fetch_failed":       func(i interface{}) *color.Escape { return color.BgRed(color.Bold(color.Blue(i))) },     // Red on Blue
	"branch_ahead":       func(i interface{}) *color.Escape { return color.BgYellow(color.Bold(color.Green(i))) }, // Green on Yellow
	"branch_behind":      func(i interface{}) *color.Escape { return color.BgYellow(color.Bold(color.Red(i))) },   // Red on Yellow
}

// Struct returned by gls go-routines
type Project struct {
	Name, State string
}

type Projects []*Project

func (projects Projects) Len() int      { return len(projects) }
func (projects Projects) Swap(i, j int) { projects[i], projects[j] = projects[j], projects[i] }

type ByName struct{ Projects }

func (s ByName) Less(i, j int) bool {
	return strings.ToLower(s.Projects[i].Name) < strings.ToLower(s.Projects[j].Name)
}

var (
	cleanGitRegex = regexp.MustCompile("nothing to commit")
	fetchErrors   = regexp.MustCompile("^fatal")
	branchAhead   = regexp.MustCompile("branch is ahead of")
	branchBehind  = regexp.MustCompile("branch is behind")
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "print help message")
	flag.Parse()
	if help {
		for k, v := range colorMap {
			fmt.Println(v(k))
		}
		return
	}
	files, err := filepath.Glob("*")
	if err != nil {
		panic(err)
	}
	glsResults := make(chan Project, 1000)

	var projects Projects
	for _, file := range files {
		file_info, _ := os.Stat(file)
		if file_info.IsDir() {
			wg.Add(1)
			go gls(file, glsResults)
		} else {
			projects = append(projects, &Project{Name: file, State: "ok"})
		}
	}
	wg.Wait()
	close(glsResults)

	for res := range glsResults {
		// make a copy to add to []projects, because res always points to the same address space
		toAppend := res
		toAppend.Name = filepath.Base(res.Name)
		projects = append(projects, &toAppend)
	}
	sort.Sort(ByName{projects})

	var projectsString string
	for _, p := range projects {
		if p.State == "ok" {
			projectsString = fmt.Sprintf("%s\t%s", projectsString, p.Name)
		} else {
			projectsString = fmt.Sprintf("%s\t%s", projectsString, colorMap[p.State](p.Name))
		}
	}

	w := columnswriter.New(os.Stdout, '\t', 0, 2)
	fmt.Fprint(w, projectsString)
	w.Flush()
}

func gls(dirName string, result chan Project) {
	defer wg.Done()
	var ret Project = Project{Name: dirName}

	// First chek, is the directory under (git) version control
	if ok, _ := exists(filepath.Join(dirName, ".git")); !ok {
		ret.State = "no_version_control"
		result <- ret
		return
	}

	gitDir := fmt.Sprintf("--git-dir=%s", filepath.Join(dirName, ".git"))
	gitTree := fmt.Sprintf("--work-tree=%s", dirName)
	output, err := exec.Command("git", gitDir, gitTree, "status").Output() //, gitDir, gitTree, "status")
	if err != nil {
		panic(err)
	}
	// Are there uncommitted changes is the directory (dirty)
	if !cleanGitRegex.MatchString(strings.TrimSpace(string(output))) {
		ret.State = "dirty"
		result <- ret
		return
	}

	// Check if the repo has a remote
	output, err = exec.Command("git", gitDir, gitTree, "remote", "-v").Output()
	if err != nil {
		panic(err)
	}
	if len(output) == 0 {
		ret.State = "no_remote"
		result <- ret
		return
	}

	// Fetch latest changes from remote
	output, err = exec.Command("git", gitDir, gitTree, "fetch").Output()
	if err != nil {
		ret.State = "fetch_failed"
		result <- ret
		return
	}
	outputStr := strings.TrimSpace(string(output))
	if fetchErrors.MatchString(outputStr) {
		ret.State = "fetch_failed"
		result <- ret
		return
	}

	output, err = exec.Command("git", gitDir, gitTree, "status").Output()
	if err != nil {
		panic(err)
	}
	outputStr = strings.TrimSpace(string(output))

	// Is branch ahead of behind of remote
	if branchAhead.MatchString(outputStr) {
		ret.State = "branch_ahead"
		result <- ret
		return
	} else if branchBehind.MatchString(outputStr) {
		ret.State = "branch_behind"
		result <- ret
		return
	}

	ret.State = "ok"
	result <- ret
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
