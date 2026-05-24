package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "log" {
		data, _ := os.ReadFile("demon.yaml")
		var cfg Config
		yaml.Unmarshal(data, &cfg)
		if cfg.Name == "" {
			fmt.Println("Error: no name in demon.yaml")
			os.Exit(1)
		}
		logpath := "/var/log/" + cfg.Name + ".log"
		cmd := exec.Command("tail", "-f", logpath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		return
	}

	data, err := os.ReadFile("demon.yaml")
	if err != nil {
		fmt.Println("Error: demon.yaml not found in current directory")
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Println("Error parsing demon.yaml:", err)
		os.Exit(1)
	}

	if cfg.Name == "" || cfg.Command == "" {
		fmt.Println("Error: name and command required in demon.yaml")
		os.Exit(1)
	}

	workdir, _ := os.Getwd()
	cmdPath := cfg.Command

	if strings.HasPrefix(cmdPath, "./") || strings.HasPrefix(cmdPath, "../") {
		cmdPath = filepath.Join(workdir, cmdPath)
	}

	script := fmt.Sprintf(`#!/bin/sh
# PROVIDE: %s
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name=%s
rcvar=%s_enable

command="/usr/sbin/daemon"
command_args="-r -p /var/run/%s.pid -o /var/log/%s.log -m 3 %s"

load_rc_config $name
: ${%s_enable:="NO"}

run_rc_command "$1"
`, cfg.Name, cfg.Name, cfg.Name, cfg.Name, cfg.Name, cmdPath, cfg.Name)

	target := "/usr/local/etc/rc.d/" + cfg.Name
	if err := os.WriteFile(target, []byte(script), 0755); err != nil {
		fmt.Printf("Write error %s: %v (run as root/sudo)\n", target, err)
		fmt.Println("\n--- script ---\n" + script)
		os.Exit(1)
	}

	fmt.Printf("Created %s (+x)\n", target)
	exec.Command("sysrc", cfg.Name+"_enable=YES").Run()
	exec.Command("service", cfg.Name, "start").Run()
	fmt.Printf("Enabled and started %s\n", cfg.Name)
}
