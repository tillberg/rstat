package main

import (
	"os"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

var pathSeparatorString = string(os.PathSeparator)

type Ignores struct {
	Specs *DirIgnores
}

type DirIgnores struct {
	Ignores []*ignore.GitIgnore
	SubDirs map[string]*DirIgnores
}

func NewDirIgnores() *DirIgnores {
	return &DirIgnores{SubDirs: map[string]*DirIgnores{}}
}

func (i *Ignores) AddIgnoreAtDir(dir string, ignore *ignore.GitIgnore) {
	specs := i.Specs
	parts := strings.Split(dir, pathSeparatorString)
	for _, part := range parts {
		subSpecs, ok := specs.SubDirs[part]
		if !ok {
			subSpecs = &DirIgnores{SubDirs: map[string]*DirIgnores{}}
			specs.SubDirs[part] = subSpecs
		}
		specs = subSpecs
	}
	specs.Ignores = append(specs.Ignores, ignore)
}

func (i *Ignores) Ignore(path string) bool {
	specs := i.Specs
	for {
		for _, ignore := range specs.Ignores {
			if ignore.MatchesPath(path) {
				return true
			}
		}
		dir, rest, _ := strings.Cut(path, pathSeparatorString)
		var ok bool
		specs, ok = specs.SubDirs[dir]
		if !ok {
			return false
		}
		path = rest
	}
}
