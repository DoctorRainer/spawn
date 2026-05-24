package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

const scriptTemplate = `#!/bin/sh
# PROVIDE: {{.Name}}
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name={{.Name}}
rcvar={{.Name}}_enable

command="/usr/sbin/daemon"
command_args="-r -p /var/run/{{.Name}}.pid -o /var/log/{{.Name}}.log {{.Command}}"

load_rc_config $name
: ${ {{.Name}}_enable:="NO"}

run_rc_command "$1"
`

func main() {
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

	tmpl, _ := template.New("rc").Parse(scriptTemplate)
	var buf bytes.Buffer
	tmpl.Execute(&buf, cfg)

	target := "/usr/local/etc/rc.d/" + cfg.Name
	if err := os.WriteFile(target, buf.Bytes(), 0755); err != nil {
		fmt.Printf("Write error %s: %v (run as root/sudo)\n", target, err)
		fmt.Println("\n--- script ---\n" + buf.String())
		os.Exit(1)
	}

	fmt.Printf("Created %s (+x)\n", target)

	exec.Command("sysrc", cfg.Name+"_enable=YES").Run()
	exec.Command("service", cfg.Name, "start").Run()

	fmt.Printf("Enabled and started %s\n", cfg.Name)
}
