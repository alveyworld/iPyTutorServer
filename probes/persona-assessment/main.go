// Command probe runs simulated students (personas) through complete D5 assessments,
// one message at a time, so the personas can be played from a Claude Code session.
//
//	probe serve                                  start a local D5 server (leave running)
//	probe start -persona competent -week 5       begin a run; prints the opening
//	probe say -run ID < answer.txt               send one persona message; prints the reply
//	probe show -run ID                           print a run's transcript
//	probe judge -run ID < judgement.json         record the judgement; reveals engine labels
//	probe report                                 combine every saved run into report.md
//
// See PLAYBOOK.md for how a run is played and judged.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// probeDir is the directory holding personas/ and results/: the executable's directory
// when built in place, otherwise the working directory.
func probeDir() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "personas")); err == nil {
			return dir
		}
	}
	wd, _ := os.Getwd()
	return wd
}

func resultsDir() string { return filepath.Join(probeDir(), "results") }
func runsDir() string    { return filepath.Join(resultsDir(), "runs") }

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(args)
	case "start":
		err = cmdStart(args)
	case "say":
		err = cmdSay(args)
	case "show":
		err = cmdShow(args)
	case "judge":
		err = cmdJudge(args)
	case "report":
		err = cmdReport(args)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: probe serve|start|say|show|judge|report [flags]   (see PLAYBOOK.md)")
	os.Exit(2)
}
