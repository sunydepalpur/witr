package model

// SocketInfo holds information about a socket's state
type SocketInfo struct {
	Port        int
	State       string // LISTEN, TIME_WAIT, CLOSE_WAIT, ESTABLISHED, etc.
	LocalAddr   string
	RemoteAddr  string
	Explanation string // Human-readable explanation of the state
	Workaround  string // Suggested workaround if applicable
}

// Connection represents a network connection (TCP or UDP)
type Connection struct {
	Protocol   string // TCP or UDP
	LocalAddr  string
	LocalPort  int
	RemoteAddr string
	RemotePort int
	State      string
	PID        int
	Process    string
}
