package main

import (
	"os/exec"
)

var path string = "/tmp/temp/data/"

func printer() {
	screen("image.jpg")
	move("image.jpg")
}

func screen(string) {
	cmd := exec.Command("spectacle", "--fullscreen", "--background", "--nonotify", "--output", "image.jpg")
	cmd.Run()
}

func move(filename string) {
	cmd := exec.Command("mv", filename, path)
	cmd.Run()
}
