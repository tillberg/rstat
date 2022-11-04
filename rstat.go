package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tillberg/alog"
)

func main() {
	alog.SetPrefix("")
	root, err := os.Getwd()
	alog.BailIf(err)
	// actualRoot preserves specified / suffixes in order to correctly resolve a root symlink
	actualRoot := filepath.Clean(root)
	if len(os.Args) > 1 {
		specPath := os.Args[1]
		if filepath.IsAbs(specPath) {
			root = specPath
		} else {
			root = filepath.Join(root, specPath)
		}
		root = filepath.Clean(root)
		actualRoot = root
		if strings.HasSuffix(specPath, "/") {
			actualRoot += "/"
		}
	}
	cumDir := map[string]*aggregator{}
	// selfDir := map[string]*agg{}
	err = filepath.Walk(actualRoot, func(path string, info os.FileInfo, err error) error {
		cumPath := path
		if err == nil && info.IsDir() == false {
			cumPath = filepath.Dir(cumPath)
		}
		for {
			agg, ok := cumDir[cumPath]
			if !ok {
				agg = &aggregator{path: cumPath}
				cumDir[cumPath] = agg
			}
			agg.add(info, err)
			if cumPath == root {
				break
			}
			if cumPath == "/" || cumPath == "." {
				alog.Panicf("parent directory iteration failed to terminate for %q\n", path)
			}
			cumPath = filepath.Dir(cumPath)
		}
		return nil
	})
	alog.BailIf(err)

	rootAgg := cumDir[root]
	sc := newScoreComputer(rootAgg)
	var interesting []*aggregator
	var allAgg []*aggregator
	scores := map[string]float64{}
	for _, agg := range cumDir {
		if agg.path == root {
			interesting = append(interesting, agg)
			continue
		}
		score := sc.computeScore(agg)
		if score >= interestingnessThreshold {
			scores[agg.path] = sc.computeScore(agg)
			allAgg = append(allAgg, agg)
		}
	}
	for {
		bestDescendantScores := map[string]float64{}
		for _, agg := range allAgg {
			p := agg.path
			for p != root {
				p = filepath.Dir(p)
				if scores[agg.path] > bestDescendantScores[p] {
					bestDescendantScores[p] = scores[agg.path]
				}
			}
		}
		bestScore := interestingnessThreshold
		var bestAgg *aggregator
		for _, agg := range allAgg {
			adjScore := scores[agg.path] - specificityPreference*bestDescendantScores[agg.path]
			if adjScore > bestScore {
				bestScore = adjScore
				bestAgg = agg
			}
		}
		if bestAgg == nil {
			break
		}
		interesting = append(interesting, bestAgg)
		p := bestAgg.path
		for p != root {
			p = filepath.Dir(p)
			scores[p] -= scores[bestAgg.path]
		}
		var newAllAgg []*aggregator
		for _, agg := range allAgg {
			if !strings.HasPrefix(agg.path, bestAgg.path) {
				newAllAgg = append(newAllAgg, agg)
			}
		}
		allAgg = newAllAgg
	}

	var rows [][]string
	rows = append(rows, []string{"", "Dirs", "Files", "Bytes"})
	colWidth := make([]int, 4)
	leftPad := []bool{false, true, true, true}
	// for i := len(interesting) - 1; i >= 0; i-- {
	for i := 0; i < len(interesting); i++ {
		rows = append(rows, formatSummary(interesting[i], rootAgg, sc))
	}
	for _, row := range rows {
		for i, col := range row {
			if colWidth[i] < alog.VisibleStringLen([]byte(col)) {
				colWidth[i] = alog.VisibleStringLen([]byte(col))
			}
		}
	}
	for rowNum, row := range rows {
		var buf bytes.Buffer
		for i, col := range row {
			buf.WriteString(" ")
			length := alog.VisibleStringLen([]byte(col))
			totalPad := colWidth[i] - length
			leftPadBytes := 0
			rightPadBytes := 0
			if rowNum == 0 {
				leftPadBytes = totalPad / 2
				rightPadBytes = totalPad - leftPadBytes
			} else if leftPad[i] {
				leftPadBytes = totalPad
			} else {
				rightPadBytes = totalPad
			}
			if leftPadBytes > 0 {
				buf.Write(bytes.Repeat([]byte(" "), leftPadBytes))
			}
			buf.WriteString(col)
			if rightPadBytes > 0 {
				buf.Write(bytes.Repeat([]byte(" "), rightPadBytes))
			}
		}
		buf.WriteString("\n")
		os.Stderr.Write(buf.Bytes())
	}
	alog.Println()
}

type aggregator struct {
	path     string
	dirs     int64
	errors   int64
	files    int64
	bytes    int64
	firstErr error
}

func (a *aggregator) add(info os.FileInfo, err error) {
	if err != nil {
		a.errors++
		if a.firstErr == nil {
			a.firstErr = err
		}
		return
	}
	a.bytes += info.Size()
	if info.IsDir() {
		a.dirs++
	} else {
		a.files++
	}
}

const (
	interestingDirCount   = 256
	interestingErrorCount = 1
	interestingFileCount  = 1024
	interestingByteCount  = 10 * 1024 * 1024

	specificityPreference    = 1.5
	interestingnessThreshold = 0.05
)

type scoreComputer struct {
	normRoot *aggregator
}

func newScoreComputer(root *aggregator) *scoreComputer {
	normRoot := &aggregator{}
	*normRoot = *root
	if normRoot.dirs < interestingDirCount {
		normRoot.dirs = interestingDirCount
	}
	if normRoot.errors < interestingErrorCount {
		normRoot.errors = interestingErrorCount
	}
	if normRoot.files < interestingFileCount {
		normRoot.files = interestingFileCount
	}
	if normRoot.bytes < interestingByteCount {
		normRoot.bytes = interestingByteCount
	}
	return &scoreComputer{
		normRoot: normRoot,
	}
}

func (c *scoreComputer) computeScore(agg *aggregator) float64 {
	var score float64
	score += float64(agg.dirs) / float64(c.normRoot.dirs)
	score += float64(agg.errors) / float64(c.normRoot.errors)
	score += float64(agg.files) / float64(c.normRoot.files)
	score += float64(agg.bytes) / float64(c.normRoot.bytes)
	return score
}

var (
	pathFormatTotal = alog.Colorify("@(green:%s) @(dim)(total)@(r)")
	pathFormat      = alog.Colorify("@(green:%s)")
	numPctFormat    = alog.Colorify(" @(cyan:%s)@(dim:% 4.0f%%) ")
	numFormat       = alog.Colorify(" @(cyan:%s)      ")
)

func formatSummary(agg *aggregator, root *aggregator, sc *scoreComputer) []string {
	if agg.errors != 0 {
		alog.Printf("@(warn:Encountered %d errors within %q. First error: %v)\n", agg.errors, agg.path, agg.firstErr)
	}
	totalScore := sc.computeScore(agg)
	var parts []string
	if agg == root {
		parts = append(parts, fmt.Sprintf(pathFormatTotal, agg.path))
	} else {
		parts = append(parts, fmt.Sprintf(pathFormat, agg.path))
	}
	appendStats := func(numFormatter func(int64) string, aggNum, rootNum, normNum int64) {
		scorePart := float64(aggNum) / float64(normNum)
		numStr := numFormatter(aggNum)
		percent := 100 * float64(aggNum) / float64(rootNum)
		var part string
		if scorePart > 0.01*totalScore && percent >= 0.5 && agg != root {
			part = fmt.Sprintf(numPctFormat, numStr, percent)
		} else {
			part = fmt.Sprintf(numFormat, numStr)
		}
		parts = append(parts, part)
	}
	appendStats(formatInt64, agg.dirs, root.dirs, sc.normRoot.dirs)
	appendStats(formatInt64, agg.files, root.files, sc.normRoot.files)
	appendStats(formatBytes, agg.bytes, root.bytes, sc.normRoot.bytes)
	return parts
}

func formatInt64(num int64) string {
	return strconv.FormatInt(num, 10)
}

var humanByteScales = []string{"", "k", "M", "G", "T", "P"}

func formatBytes(num int64) string {
	fnum := float64(num)
	scale := 0
	for fnum > 1000 {
		if scale+1 >= len(humanByteScales) {
			break
		}
		scale++
		fnum /= 1000
	}
	decPlaces := 0
	if scale > 0 {
		if fnum < 10 {
			decPlaces = 2
		} else {
			decPlaces = 1
		}
	}
	numStr := strconv.FormatFloat(fnum, 'f', decPlaces, 64)
	return fmt.Sprintf("%s%s", numStr, humanByteScales[scale])
}
