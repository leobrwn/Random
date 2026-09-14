package main

import (
	"os"
	"os/exec"
)

var output string = "/tmp/temp/data/prosses_info.txt"

func getinfo() {
	var result string

	result += "TOP 20 PROCESSES BY CPU\n\n"
	cpu, _ := exec.Command("sh", "-c,", "ps", "-eo", "pid,comm,pcpu,pmem", "--sort=-pcpu", "|", "head", "20").Output()
	result += string(cpu)

	result += "\nTOP 20 PROCESSES BY MEMORY\n\n"
	mem, _ := exec.Command("sh", "-c,", "ps", "-eo", "pid,comm,pcpu,pmem", "--sort=-pmem", "|", "head", "20").Output()
	result += string(mem)

	os.WriteFile(output, []byte(result), 0644)
}

func procinfo() {
	getinfo()
}
