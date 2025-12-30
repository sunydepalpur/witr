//go:build linux

package proc

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/pranshuparmar/witr/pkg/model"
)

func readListeningSockets() (map[string]Socket, error) {
	sockets := make(map[string]Socket)

	parse := func(path string, ipv6 bool) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Scan() // skip header

		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			local := fields[1]
			state := fields[3]
			inode := fields[9]

			// 0A = LISTEN
			if state != "0A" {
				continue
			}

			addr, port := parseAddr(local, ipv6)
			sockets[inode] = Socket{
				Inode:   inode,
				Port:    port,
				Address: addr,
			}
		}
	}

	parse("/proc/net/tcp", false)
	parse("/proc/net/tcp6", true)

	return sockets, nil
}

func parseAddr(raw string, ipv6 bool) (string, int) {
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return "", 0
	}
	portHex := parts[1]
	port, _ := strconv.ParseInt(portHex, 16, 32)

	ipHex := parts[0]
	b, err := hex.DecodeString(ipHex)
	if err != nil {
		return "", int(port)
	}

	if ipv6 {
		if len(b) != 16 {
			return "::", int(port)
		}
		// /proc/net/tcp6 stores IPv6 as 4 little-endian 32-bit groups
		// Reverse bytes within each 4-byte group
		ip := make(net.IP, 16)
		for i := 0; i < 4; i++ {
			ip[i*4+0] = b[i*4+3]
			ip[i*4+1] = b[i*4+2]
			ip[i*4+2] = b[i*4+1]
			ip[i*4+3] = b[i*4+0]
		}
		return ip.String(), int(port)
	}

	if len(b) < 4 {
		return "", int(port)
	}
	ip := strconv.Itoa(int(b[3])) + "." +
		strconv.Itoa(int(b[2])) + "." +
		strconv.Itoa(int(b[1])) + "." +
		strconv.Itoa(int(b[0]))

	return ip, int(port)
}

func GetAllConnections() ([]model.Connection, error) {
	var connections []model.Connection

	// Map inodes to PIDs
	inodeMap := make(map[string]int)
	files, _ := os.ReadDir("/proc")
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(f.Name())
		if err != nil {
			continue
		}

		fdPath := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(fdPath + "/" + fd.Name())
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") {
				inode := link[8 : len(link)-1]
				inodeMap[inode] = pid
			}
		}
	}

	parse := func(path string, protocol string, ipv6 bool) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Scan() // skip header

		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			local := fields[1]
			remote := fields[2]
			state := fields[3]
			inode := fields[9]

			localIP, localPort := parseAddr(local, ipv6)
			remoteIP, remotePort := parseAddr(remote, ipv6)

			pid := inodeMap[inode]
			procName := ""
			if pid > 0 {
				procName = GetCmdline(pid)
				if idx := strings.Index(procName, " "); idx != -1 {
					procName = procName[:idx]
				}
				if idx := strings.LastIndex(procName, "/"); idx != -1 {
					procName = procName[idx+1:]
				}
			}

			stateStr := ""
			if protocol == "TCP" {
				stateVal, _ := strconv.ParseInt(state, 16, 64)
				stateStr = mapTCPState(int(stateVal))
			}

			connections = append(connections, model.Connection{
				Protocol:   protocol,
				LocalAddr:  localIP,
				LocalPort:  localPort,
				RemoteAddr: remoteIP,
				RemotePort: remotePort,
				State:      stateStr,
				PID:        pid,
				Process:    procName,
			})
		}
	}

	parse("/proc/net/tcp", "TCP", false)
	parse("/proc/net/tcp6", "TCP", true)
	parse("/proc/net/udp", "UDP", false)
	parse("/proc/net/udp6", "UDP", true)

	return connections, nil
}
