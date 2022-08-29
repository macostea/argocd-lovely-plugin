package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/otiai10/copy"
)

var processors = []Processor{
	helmProcessor{},
	kustomizeProcessor{},
	yamlProcessor{},
	pluginProcessor{},
}

// Collection is a list of sub-applications making up this application
type Collection struct {
	dirs PackageDirectories
}

func (c *Collection) scanFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if info.IsDir() {
		if c.dirs.KnownSubDirectory(path) {
			// We don't allow subdirectories of paths with yaml in
			// to be packages in their own right
			return filepath.SkipDir
		}
		return nil
	}
	yamlRegexp := regexp.MustCompile(`\.ya?ml$`)
	dir := filepath.Dir(path)
	if yamlRegexp.MatchString(path) {
		c.dirs.AddDirectory(dir)
	}
	return nil
}

func (c *Collection) scanDir(path string) error {
	return filepath.Walk(path, c.scanFile)
}

func (c *Collection) processAllDirs() (string, error) {
	result := ""
	for _, path := range c.dirs.GetPackages() {
		output, err := c.processOneDir(path)
		if err != nil {
			return "", err
		}
		result += output
	}
	return result, nil
}

func (c *Collection) processOneDir(path string) (string, error) {
	var result *string
	pre := preProcessor{}
	if pre.enabled(path) {
		err := pre.generate(path)
		if err != nil {
			return "", err
		}
	}
	for _, processor := range processors {
		if processor.enabled(path) {
			out, err := processor.generate(result, path)
			if err != nil {
				return "", err
			}
			result = out
		}
	}
	return *result, nil
}

// We copy the directory in case we patch some of the files for kustomize or helm
// ArgoCD doesn't guarantee us an unpatched copy when we run
func (c *Collection) makeTmpCopy(path string) (string, error) {
	tmpPath, err := ioutil.TempDir(os.TempDir(), "lovely-plugin-")
	if err != nil {
		return tmpPath, err
	}
	err = os.RemoveAll(tmpPath)
	if err != nil {
		return tmpPath, err
	}
	err = copy.Copy(path, tmpPath)
	return tmpPath, err
}

func (c *Collection) doAllDirs(path string) (string, error) {
	err := c.scanDir(path)
	if err != nil {
		log.Fatal(err)
	}
	output, err := c.processAllDirs()
	if err != nil {
		log.Fatal(err)
	}
	return output, err
}

func parseArgs() (bool, error) {
	if len(os.Args[1:]) == 0 {
		return false, nil
	}
	if len(os.Args[1:]) > 1 {
		return false, errors.New("Too many arguments. Only one optional argument allowed of 'init'")
	}
	if os.Args[1] == `init` {
		return true, nil
	}
	return false, errors.New("Invalid argument. Only one optional argument allowed of 'init'")
}

func main() {
	initMode, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}
	if initMode {
		return
	}
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	c := Collection{}

	workingPath, err := c.makeTmpCopy(dir)
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(workingPath)

	subdir := KustomizeOverlayDir()
	if subdir != "" {
		workingPath += "/" + subdir
	}

	output, err := c.doAllDirs(workingPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(output)
}
