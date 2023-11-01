package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sort"
	"os/exec"
	"bytes"
)

type IProj interface {
	GetName() string
	GetFiles() []IncludedFile
}

type IncludedFile struct {
	Include string `xml:"Include,attr"`
}

type Proj struct {
	TargetName        string `xml:"PropertyGroup>TargetName"`
	ConfigurationType string `xml:"PropertyGroup>ConfigurationType"`
	ProjectName       string `xml:"PropertyGroup>ProjectName"`
	RootNamespace     string `xml:"PropertyGroup>RootNamespace"`
	Filename          string
	Files		  []IncludedFile `xml:"ItemGroup>ClCompile"`
}

func (p Proj) GetName() string {
	if filepath.Base(p.Filename) == "dirs.proj" {
		return "dir dir"
	}
	name := p.TargetName
	if len(name) < 1 {
		name = p.ProjectName
	}
	if len(name) < 1 {
		name = p.RootNamespace
	}
	if len(name) < 1 {
		name = filepath.Base(p.Filename)
		var ext = filepath.Ext(p.Filename)
		name = name[0 : len(name)-len(ext)]
	}
	if p.ConfigurationType == "StaticLibrary" {
		name = name + " lib"
	} else if p.ConfigurationType == "DynamicLibrary" {
		name = name + " dll"
	} else if p.ConfigurationType == "Library" {
		name = name + " dll"
	} else if p.ConfigurationType == "Application" {
		name = name + " exe"
	} else if p.ConfigurationType == "Driver" {
		name = name + " drv"
	} else {
		name = name + " unknown"
	}
	return name
}

func (p Proj) GetFiles() []IncludedFile {
	return p.Files
}

func ParseProject(fpath string) (IProj, error) {
	var p Proj
	p.Filename = fpath
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if err = xml.Unmarshal(content, &p); err != nil {
		return nil, err
	}
	return p, nil
}

func WalkMatch(root string, exts []string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == "obj" || info.Name() == "objd" {
				return filepath.SkipDir
			}
			return nil
		}
		matched := false
		fileext := filepath.Ext(path)
		for _, ext := range exts {
			if fileext == ext {
				matched = true
				break
			}
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func PrintProjects(prj <- chan string) {
	for {
		if line, ok := <- prj; ok {
			fmt.Println(line)
		} else {
			break
		}
	}
}

func PreProcessProject(projfile string) error {
	tmpdir, err := os.MkdirTemp("", "msbuild*")
	if err != nil {
		return err
	}
	matching, err := filepath.Glob(projfile)
	if err == nil {
		fmt.Println("Matching:", matching)
	}
	fi, err := ioutil.ReadDir(filepath.Dir(projfile))
	if err == nil {
		for _, f := range fi {
			fmt.Println("Files:", f.Name())
		}
	}
	// defer os.RemoveAll(tmpdir)
	preprocessed := filepath.Join(tmpdir, "preprocessed.vcxproj")
	fmt.Println("Preprocessing", projfile, "to", preprocessed)
	c := exec.Command("msbuild", "/pp:" + preprocessed, filepath.Base(projfile))
	c.Dir = filepath.Dir(projfile)
	var outb, errb bytes.Buffer
	c.Stdout = &outb
	c.Stderr = &errb
	err = c.Run()
	if err != nil {
		fmt.Println(errb.String())
		return err
	}
	p, err := ParseProject(preprocessed)
	if err != nil {
		return err
	}
	fmt.Println(p.GetName(), p.GetFiles())
	return nil
}

func main() {
	err := PreProcessProject(os.Args[1])
	if err != nil {
		fmt.Println(err)
		if false {
			files, err := WalkMatch("client/onedrive", []string{".vcxproj", ".csproj", ".proj"})
			if err != nil {
				panic("Can't find project files")
			}
			sort.Strings(files)
			projects := make(chan string)
			go PrintProjects(projects)
			defer func() {
				close(projects)
			}()

			wg := sync.WaitGroup{}
			for _, file := range files {
				wg.Add(1)
				go func(file string){
					p, err := ParseProject(file)
					if err == nil {
						projects <- p.GetName()+" "+filepath.Dir(file)
					}
					wg.Done()
				}(file)
			}
			wg.Wait()
		}
	}	
}
