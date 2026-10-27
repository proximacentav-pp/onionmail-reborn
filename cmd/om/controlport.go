package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"onionmail/internal/omcore"
)

func controlportCmd(args []string) int {
	usage := func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  om controlport rootpass --set <password>")
		fmt.Fprintln(os.Stderr, "  om controlport port --set <port>")
		fmt.Fprintln(os.Stderr, "  om controlport port --show")
		fmt.Fprintln(os.Stderr, "  om controlport enable|disable")
		fmt.Fprintln(os.Stderr, "  om controlport ssh enable|disable")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-port --set <port> | --show")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-user --set <user> | --show")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-mode sshtunnel|unactive")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-auth password|key")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-algo <algorithm>")
		fmt.Fprintln(os.Stderr, "  om controlport ssh-key --show | --set <public-key> | --rm")
		fmt.Fprintln(os.Stderr, "  om controlport status")
	}
	if len(args) == 0 {
		return controlportStatus()
	}
	if args[0] == "-h" || args[0] == "--help" {
		usage()
		return 0
	}
	st := omcore.LoadState()
	ctrl := omcore.Obj(st, "control")
	changed := false
	switch args[0] {
	case "rootpass":
		v, ok := optionValue(args[1:], "--set")
		if !ok {
			usage()
			return 1
		}
		if !regexp.MustCompile(`^[!-~]{8,128}$`).MatchString(v) {
			fmt.Fprintln(os.Stderr, "om controlport: password must be 8-128 printable characters without spaces")
			return 1
		}
		ctrl["root_password"] = v
		changed = true
	case "port":
		if len(args) > 1 && args[1] == "--show" {
			fmt.Println(omcore.ControlPort())
			return 0
		}
		v, ok := optionValue(args[1:], "--set")
		if !ok {
			usage()
			return 1
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1024 || n > 65535 {
			fmt.Fprintln(os.Stderr, "om controlport: port must be 1024..65535")
			return 1
		}
		ctrl["port"] = n
		changed = true
	case "enable":
		ctrl["enabled"] = true
		changed = true
	case "disable":
		ctrl["enabled"] = false
		changed = true
	case "ssh":
		if len(args) < 2 {
			usage()
			return 1
		}
		switch args[1] {
		case "enable":
			ctrl["ssh_enabled"] = true
			changed = true
		case "disable":
			ctrl["ssh_enabled"] = false
			ctrl["ssh_mode"] = "unactive"
			changed = true
		default:
			usage()
			return 1
		}
	case "ssh-port":
		if len(args) > 1 && args[1] == "--show" {
			_, p, _, _ := omcore.ControlSSHFromState(st)
			fmt.Println(p)
			return 0
		}
		v, ok := optionValue(args[1:], "--set")
		if !ok {
			usage()
			return 1
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1024 || n > 65535 {
			fmt.Fprintln(os.Stderr, "om controlport: encrypted port must be 1024..65535")
			return 1
		}
		ctrl["ssh_port"] = n
		changed = true
	case "ssh-user":
		if len(args) > 1 && args[1] == "--show" {
			_, _, u, _ := omcore.ControlSSHFromState(st)
			fmt.Println(u)
			return 0
		}
		v, ok := optionValue(args[1:], "--set")
		if !ok {
			usage()
			return 1
		}
		ctrl["ssh_user"] = strings.ToLower(strings.TrimSpace(v))
		changed = true
	case "ssh-mode":
		if len(args) < 2 {
			usage()
			return 1
		}
		switch args[1] {
		case "sshtunnel":
			ctrl["ssh_mode"] = "sshtunnel"
			ctrl["ssh_enabled"] = true
			changed = true
		case "unactive", "off":
			ctrl["ssh_mode"] = "unactive"
			changed = true
		default:
			fmt.Fprintln(os.Stderr, "om controlport: ssh-mode must be sshtunnel or unactive")
			return 1
		}
	case "ssh-auth":
		if len(args) < 2 || (args[1] != "password" && args[1] != "key") {
			fmt.Fprintln(os.Stderr, "om controlport: ssh-auth must be password or key")
			return 1
		}
		ctrl["ssh_auth"] = args[1]
		changed = true
	case "ssh-algo":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "om controlport: ssh-algo requires an algorithm")
			return 1
		}
		algo, ok := omcore.SSHAlgoByID(args[1])
		if !ok {
			fmt.Fprintln(os.Stderr, "om controlport: unknown algorithm "+args[1])
			return 1
		}
		ctrl["ssh_algo"] = algo.ID
		changed = true
		if !algo.Supported {
			fmt.Fprintf(os.Stderr, "om controlport: warning: OpenSSH does not support %s; adding a key will fail\n", algo.Label)
		}
	case "ssh-key":
		return controlportKeyCmd(st, args[1:])
	case "status", "show":
		return controlportStatus()
	default:
		fmt.Fprintf(os.Stderr, "om controlport: unknown argument %s\n", args[0])
		usage()
		return 1
	}
	if !changed {
		usage()
		return 1
	}
	st["control"] = ctrl
	ok, errs := omcore.Save(st, "")
	if !ok {
		fmt.Fprintln(os.Stderr, "om controlport: "+strings.Join(errs, "; "))
		return 1
	}
	enabled, port := omcore.ControlFromState(st)
	sshEnabled, sshPort, sshUser, sshMode := omcore.ControlSSHFromState(st)
	sshAuth, sshAlgo := omcore.ControlSSHAuthFromState(st)
	fmt.Printf("ControlPort: %s, port %d\n", onoff(enabled), port)
	fmt.Printf("Encrypted:   %s, port %d, user %s, telnet %s\n", onoff(sshEnabled), sshPort, sshUser, sshMode)
	fmt.Printf("SSH auth:    %s, algorithm %s\n", sshAuth, omcore.SSHAlgoLabel(sshAlgo))
	if len(omcore.OnionmailPIDs()) == 0 {
		fmt.Println("OnionMail is not running. Changes will apply at the next start.")
		return 0
	}
	fmt.Println("Restarting OnionMail to apply the changes...")
	omcore.Restart()
	return 0
}

func controlportKeyCmd(st omcore.State, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "om controlport: ssh-key needs --show, --set or --rm")
		return 1
	}
	switch args[0] {
	case "--show":
		if fp, ok := omcore.ControlSSHClientKeyInfo(); ok {
			fmt.Println(fp)
		} else {
			fmt.Println("none")
		}
		return 0
	case "--set":
		v, ok := optionValue(args, "--set")
		if !ok || strings.TrimSpace(v) == "" {
			fmt.Fprintln(os.Stderr, "om controlport: ssh-key --set requires a public key")
			return 1
		}
		if err := omcore.ControlSSHAddClientKey(st, v); err != nil {
			fmt.Fprintln(os.Stderr, "om controlport: "+err.Error())
			return 1
		}
		fp, _ := omcore.ControlSSHClientKeyInfo()
		fmt.Println("Public key added: " + fp)
		return 0
	case "--rm":
		if err := omcore.ControlSSHRemoveClientKey(st); err != nil {
			fmt.Fprintln(os.Stderr, "om controlport: "+err.Error())
			return 1
		}
		fmt.Println("Public key removed")
		return 0
	default:
		fmt.Fprintln(os.Stderr, "om controlport: ssh-key needs --show, --set or --rm")
		return 1
	}
}

func controlportStatus() int {
	st := omcore.LoadState()
	enabled, port := omcore.ControlFromState(st)
	sshEnabled, sshPort, sshUser, sshMode := omcore.ControlSSHFromState(st)
	sshAuth, sshAlgo := omcore.ControlSSHAuthFromState(st)
	fmt.Printf("ControlPort: %s\n", onoff(enabled))
	fmt.Printf("Port:        %d\n", port)
	fmt.Printf("Encrypted:   %s\n", onoff(sshEnabled))
	fmt.Printf("SSH port:    %d\n", sshPort)
	fmt.Printf("SSH user:    %s\n", sshUser)
	fmt.Printf("Telnet mode: %s\n", sshMode)
	fmt.Printf("SSH auth:    %s\n", sshAuth)
	fmt.Printf("SSH algo:    %s\n", omcore.SSHAlgoLabel(sshAlgo))
	if fp, ok := omcore.ControlSSHClientKeyInfo(); ok {
		fmt.Printf("SSH key:     %s\n", fp)
	} else {
		fmt.Printf("SSH key:     none\n")
	}
	fmt.Printf("Login:       %s\n", omcore.StrDef(st, "nick", "onionmail"))
	fmt.Printf("RootPass:    %s\n", strings.Repeat("*", 8))
	return 0
}

func optionValue(args []string, name string) (string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == name {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), true
		}
	}
	return "", false
}
