package main

import (
	"fmt"
	"github.com/str1ngs/ansi/color"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var wg sync.WaitGroup

type colorFunc func(interface{}) *color.Escape

func blueBold(i interface{}) *color.Escape {
	return color.Bold(color.Blue(i))
}

func redBold(i interface{}) *color.Escape {
	return color.Bold(color.Red(i))
}

var colors map[string]colorFunc = map[string]colorFunc{
	"no_version_control": blueBold,
	"dirty":              redBold,
}

type item struct {
	Name, State string
}

var cleanGitRegex = regexp.MustCompile("nothing to commit")

func main() {
	files, err := filepath.Glob("../*")
	if err != nil {
		panic(err)
	}
	glsResults := make(chan item, 100)
	for _, file := range files {
		file_info, _ := os.Stat(file)
		if file_info.IsDir() {
			wg.Add(1)
			go gls(file, glsResults)
		}
	}
	wg.Wait()
	close(glsResults)

	for res := range glsResults {
		res.Name = filepath.Base(res.Name)
		if res.State == "ok" {
			fmt.Println(res.Name)
		} else {
			fmt.Printf("%s\n", colors[res.State](res.Name))
		}
	}
}

func gls(dirName string, result chan item) {
	defer wg.Done()
	var ret item = item{Name: dirName}

    // First chek, is the directory under (git) version control
	if ok, _ := exists(filepath.Join(dirName, ".git")); !ok {
		ret.State = "no_version_control"
		result <- ret
		return
	}

	gitDir := fmt.Sprintf("--git-dir=%s", filepath.Join(dirName, ".git"))
	gitTree := fmt.Sprintf("--work-tree=%s", dirName)
	cmd := exec.Command("git", gitDir, gitTree, "status") //, gitDir, gitTree, "status")
	output, err := cmd.Output()
	if err != nil {
		panic(err)
	}
    // Are there uncommitted changes is the directory (dirty)
	if !cleanGitRegex.MatchString(strings.TrimSpace(string(output))) {
		ret.State = "dirty"
		result <- ret
		return
	}

    // Fetch from remote

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
