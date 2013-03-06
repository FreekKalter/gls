package main

import (
	"flag"
	"fmt"
	"github.com/str1ngs/ansi/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var wg sync.WaitGroup

type colorFunc func(interface{}) *color.Escape

var colors map[string]colorFunc = map[string]colorFunc{
	"no_version_control": blueBold,
	"dirty":              redBold,
	"no_remote":          redOnBlue,
	"fetch_failed":       redOnBlue,
	"branch_ahead":       greenOnOrange,
	"branch_behind":      redOnOrange,
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
	fetchErrors   = regexp.MustCompile("ERROR")
	branchAhead   = regexp.MustCompile("branch is ahead of")
	branchBehind  = regexp.MustCompile("branch is behind")
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "print help message")
	flag.Parse()
	if help {
		for k, v := range colors {
			fmt.Println(v(k))
		}
		return
	}
	files, err := filepath.Glob("*")
	if err != nil {
		panic(err)
	}
	glsResults := make(chan Project, 100)
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
		toAppend := res
		toAppend.Name = filepath.Base(res.Name)

		projects = append(projects, &toAppend)
	}
	sort.Sort(ByName{projects})

	printInCollumns(projects)
}

func printProjects(i []*Project) {
	for _, v := range i {
		fmt.Printf("%s\n", v.Name)
	}
	fmt.Print("\n")
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
		panic(err)
	}
	outputStr := strings.TrimSpace(string(output))
	if fetchErrors.MatchString(outputStr) {
		ret.State = "fetch_failed"
		result <- ret
		return
	}

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

func printInCollumns(projects []*Project) {
	nrProjects := len(projects)
	nrTerminalColumnsInt32, _ := strconv.ParseInt(os.Getenv("COLUMNS"), 10, 32)
	nrTerminalColumns := int(nrTerminalColumnsInt32)
	var nrCollumns, nrRows, totalWidth int = 0, 1, 0
	for _, file := range projects {
		if (totalWidth + len(file.Name) + 1) > nrTerminalColumns {
			break
		}
		totalWidth += len(file.Name) + 2
		nrCollumns++
	}
	calcNrRows := func(projects, collumns int) int {
		return int(math.Ceil(float64(projects) / float64(collumns)))
	}
	nrRows = calcNrRows(nrProjects, nrCollumns)

	totalWidth = totalWidth * 2
	var collumnWidths []int
	for totalWidth > nrTerminalColumns {
		totalWidth = 0
		collumnWidths = []int{}
		for x := 0; x < nrCollumns; x++ {
			maxCollumnWidth := 0
			for y := 0; y < nrRows; y++ {
				index := y*nrCollumns + x
				if index >= nrProjects {
					break
				}
				if len(projects[index].Name) > maxCollumnWidth {
					maxCollumnWidth = len(projects[index].Name)
				}
			}
			totalWidth += maxCollumnWidth + 2
			collumnWidths = append(collumnWidths, maxCollumnWidth)

		}
		if totalWidth > nrTerminalColumns {
			nrCollumns--
			nrRows = calcNrRows(nrProjects, nrCollumns)
		}
	}

	for y := 0; y < nrRows; y++ {
		for x := 0; x < nrCollumns; x++ {
			index := y*nrCollumns + x
			if index >= nrProjects {
				break
			}
			var toPrint string
			if projects[index].State == "ok" {
				toPrint = projects[index].Name
			} else {
				toPrint = (colors[projects[index].State](projects[index].Name)).String()
			}

			lenDiff := 0
			if projects[index].State != "ok" {
				lenDiff = len(toPrint) - len(projects[index].Name)
			}
			if len(collumnWidths) > 0 {
				fmt.Printf("%-*s", collumnWidths[x]+lenDiff+2, toPrint)
			} else {
				fmt.Printf("%-*s", lenDiff+2, toPrint)
			}
		}
		fmt.Print("\n")
	}
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

func blueBold(i interface{}) *color.Escape {
	return color.Bold(color.Blue(i))
}

func redBold(i interface{}) *color.Escape {
	return color.Bold(color.Red(i))
}

func redOnBlue(i interface{}) *color.Escape {
	return color.BgBlue(color.Bold(color.Red(i)))
}

func redOnOrange(i interface{}) *color.Escape {
	return color.BgYellow(color.Bold(color.Red(i)))
}

func greenOnOrange(i interface{}) *color.Escape {
	return color.BgYellow(color.Bold(color.Green(i)))
}
