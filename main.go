package main

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Env     map[string]string `yaml:"env"`
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
		cmd := exec.Command("tail", "-f", "/var/log/"+cfg.Name+".log")
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

	// .env + yaml env
	envMap := make(map[string]string)
	if envFile, err := os.Open(".env"); err == nil {
		defer envFile.Close()
		scanner := bufio.NewScanner(envFile)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if before, after, ok := strings.Cut(line, "="); ok {
				envMap[strings.TrimSpace(before)] = strings.TrimSpace(after)
			}
		}
	}
	maps.Copy(envMap, cfg.Env)

	// resolve relative paths
	parts := strings.Fields(cfg.Command)
	for i, p := range parts {
		if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
			parts[i] = filepath.Join(workdir, p)
		}
	}
	cmdPath := strings.Join(parts, " ")

	// env part (only if vars exist)
	envPart := ""
	if len(envMap) > 0 {
		var envs []string
		for k, v := range envMap {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
		envPart = " env " + strings.Join(envs, " ")
	}

	script := fmt.Sprintf(`#!/bin/sh
# PROVIDE: %s
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name=%s
rcvar=%s_enable

command="/usr/sbin/daemon"
command_args="-r -f -H -P /var/run/%s.pid -o /var/log/%s.log -m 3%s %s"

load_rc_config $name
: ${%s_enable:="NO"}
run_rc_command "$1"
`, cfg.Name, cfg.Name, cfg.Name, cfg.Name, cfg.Name, envPart, cmdPath, cfg.Name)

	target := "/usr/local/etc/rc.d/" + cfg.Name
	if err := os.WriteFile(target, []byte(script), 0755); err != nil {
		fmt.Printf("Write error %s: %v (run as root/sudo)\n", target, err)
		fmt.Println("\n--- script ---\n" + script)
		os.Exit(1)
	}

	fmt.Printf("Created %s (+x)\n", target)

		// Enable
	if err := exec.Command("sysrc", cfg.Name+"_enable=YES").Run(); err != nil {
		fmt.Printf("sysrc warning: %v\n", err)
	} else {
		fmt.Printf("Enabled %s\n", cfg.Name)
	}

	// Stop
	exec.Command("service", cfg.Name, "stop").Run() // ignore output

	time.Sleep(100 * time.Millisecond)

	// Start with real error capture
	startCmd := exec.Command("service", cfg.Name, "start")
	output, err := startCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("❌ Start ERROR: %v\nOutput:\n%s\n", err, string(output))
	} else {
		fmt.Printf("✅ Started %s\n", cfg.Name)
	}
}
