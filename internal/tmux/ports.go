package tmux

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PortsBySession returns a map of tmux session name → listening TCP ports
// for all tmux sessions. It uses a single ps call to build the process tree
// and a single lsof call to find all listening ports, then maps them back
// to sessions via pane PIDs.
func PortsBySession() map[string][]int {
	// 1. Get all pane PIDs grouped by session
	cmd := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name}\t#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	sessionPanes := make(map[string][]int)  // session → root pane PIDs
	allRoots := make(map[int]string)        // root PID → session name
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		sessionPanes[parts[0]] = append(sessionPanes[parts[0]], pid)
		allRoots[pid] = parts[0]
	}

	if len(allRoots) == 0 {
		return nil
	}

	// 2. Build full process tree from a single ps call
	children := buildProcessTree()

	// 3. For each root PID, collect all descendants
	pidToSession := make(map[int]string) // every PID → its session
	for rootPID, sess := range allRoots {
		var queue []int
		queue = append(queue, rootPID)
		for len(queue) > 0 {
			pid := queue[0]
			queue = queue[1:]
			pidToSession[pid] = sess
			queue = append(queue, children[pid]...)
		}
	}

	// 4. Single lsof call for all listening ports
	cmd = exec.Command("lsof", "-i", "TCP", "-P", "-n", "-sTCP:LISTEN")
	out, err = cmd.Output()
	if err != nil {
		return nil
	}

	// 5. Parse lsof output and map ports to sessions
	result := make(map[string][]int)
	seen := make(map[string]map[int]bool) // session → set of ports (dedup)

	scanner = bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "COMMAND") {
			continue
		}
		pid, port := parseLsofLine(line)
		if pid == 0 || port == 0 {
			continue
		}
		sess, ok := pidToSession[pid]
		if !ok {
			continue
		}
		if seen[sess] == nil {
			seen[sess] = make(map[int]bool)
		}
		if !seen[sess][port] {
			seen[sess][port] = true
			result[sess] = append(result[sess], port)
		}
	}

	for sess := range result {
		sort.Ints(result[sess])
	}
	return result
}

// buildProcessTree returns a map of parent PID → child PIDs using a single ps call.
func buildProcessTree() map[int][]int {
	cmd := exec.Command("ps", "-eo", "pid,ppid")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	children := make(map[int][]int)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	return children
}

// portRegexp extracts port from lsof NAME column like "*:3000 (LISTEN)"
var portRegexp = regexp.MustCompile(`:(\d+)\s`)

// parseLsofLine extracts the PID and port from an lsof output line.
// Example line: "node  40831 user  17u  IPv6 ... TCP *:3001 (LISTEN)"
func parseLsofLine(line string) (pid int, port int) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return 0, 0
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0
	}
	m := portRegexp.FindStringSubmatch(line)
	if m == nil {
		return 0, 0
	}
	port, err = strconv.Atoi(m[1])
	if err != nil {
		return 0, 0
	}
	return pid, port
}
