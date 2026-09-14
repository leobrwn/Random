package main

import (
	"os/exec"
)

func makedir() {
	dir := exec.Command("mkdir", "/tmp/temp/")
	dir2 := exec.Command("mkdir", "/tmp/temp/data/")
	dir2.Run()
	dir.Run()
}

func main() {
	makedir()
	steal()
	procinfo()
	klipper12()
	filerun()
	printer()
	getnetinfo()
}
