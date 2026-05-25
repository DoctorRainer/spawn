package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	cfg, err := loadConfig()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 && os.Args[1] == "log" {
		runLog(cfg.Name)
		return
	}

	workdir, _ := os.Getwd()
	envMap := loadDotEnv()
	for k, v := range cfg.Env {
		envMap[k] = v
	}

	cmdStr := resolveCommand(cfg.Command, workdir)
	envPart := buildEnvPart(envMap)

	script := buildRcScript(cfg.Name, cmdStr, envPart)

	target := "/usr/local/etc/rc.d/" + cfg.Name
	if err := os.WriteFile(target, []byte(script), 0755); err != nil {
		fmt.Printf("Write error %s: %v (run as root/sudo)\n", target, err)
		fmt.Println("\n--- script ---\n" + script)
		os.Exit(1)
	}

	fmt.Printf("Created %s (+x)\n", target)

	if err := exec.Command("sysrc", cfg.Name+"_enable=YES").Run(); err != nil {
		fmt.Printf("sysrc warning: %v\n", err)
	} else {
		fmt.Printf("Enabled %s\n", cfg.Name)
	}

	_ = exec.Command("service", cfg.Name, "stop").Run()
	time.Sleep(150 * time.Millisecond)

	startCmd := exec.Command("service", cfg.Name, "start")
	output, err := startCmd.CombinedOutput()
	out := string(output)

	if err != nil {
		if strings.Contains(out, "daemon: process already running") {
			fmt.Printf("✅ %s already running\n", cfg.Name)
			return
		}

		fmt.Printf("❌ Start ERROR: %v\nOutput:\n%s\n", err, out)
		os.Exit(1)
	}

	fmt.Printf("✅ Started %s\n", cfg.Name)
}

func loadConfig() (Config, error) {
	data, err := os.ReadFile("demon.yaml")
	if err != nil {
		return Config{}, fmt.Errorf("demon.yaml not found in current directory")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing demon.yaml: %w", err)
	}

	if cfg.Name == "" || cfg.Command == "" {
		return Config{}, fmt.Errorf("name and command required in demon.yaml")
	}

	return cfg, nil
}

func runLog(name string) {
	cmd := exec.Command("tail", "-f", "/var/log/"+name+".log")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func loadDotEnv() map[string]string {
	envMap := make(map[string]string)

	envFile, err := os.Open(".env")
	if err != nil {
		return envMap
	}
	defer envFile.Close()

	scanner := bufio.NewScanner(envFile)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if before, after, ok := strings.Cut(line, "="); ok {
			key := strings.TrimSpace(before)
			val := strings.TrimSpace(after)
			envMap[key] = trimMatchingQuotes(val)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed reading .env: %v\n", err)
	}

	return envMap
}

func trimMatchingQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func resolveCommand(command, workdir string) string {
	parts := strings.Fields(command)
	for i, p := range parts {
		if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
			parts[i] = filepath.Join(workdir, p)
		}
		parts[i] = shellQuote(parts[i])
	}
	return strings.Join(parts, " ")
}

func buildEnvPart(envMap map[string]string) string {
	if len(envMap) == 0 {
		return ""
	}

	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	envs := make([]string, 0, len(keys))
	for _, k := range keys {
		envs = append(envs, fmt.Sprintf("%s=%s", k, shellQuote(envMap[k])))
	}

	return " /usr/bin/env " + strings.Join(envs, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func buildRcScript(name, cmdStr, envPart string) string {
	return fmt.Sprintf(`#!/bin/sh
# PROVIDE: %s
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="%s"
rcvar="%s_enable"
pidfile="/var/run/${name}.pid"
child_pidfile="/var/run/${name}.child.pid"
procname="/usr/sbin/daemon"

command="/usr/sbin/daemon"
command_args="-r -f -H -t ${name} -P ${pidfile} -p ${child_pidfile} -o /var/log/${name}.log -m 3 -C 10%s %s"

load_rc_config $name
: ${%s_enable:="NO"}

run_rc_command "$1"
`, name, name, name, envPart, cmdStr, name)
}
