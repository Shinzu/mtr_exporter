// mtr is a package that processes and saves
// an `mtr --raw` call's output.
package mtr

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"os/exec"
	"strconv"
)

type Host struct {
	IP              net.IP  `json:"ip"`
	Name            string  `json:"hostname"`
	Hop             int     `json:"hop-number"`
	PacketMicrosecs []int   `json:"packet-times"`
	Sent            int     `json:"sent"`
	Received        int     `json:"received"`
	Dropped         int     `json:"dropped"`
	LostPercent     float64 `json:"lost-percent"`
	// All packet units are in microseconds
	Mean               float64 `json:"mean"`
	Best               int     `json:"best"`
	Worst              int     `json:"worst"`
	StandardDev        float64 `json:"standard-dev"`
	MeanJitter         float64 `json:"mean-jitter"`
	WorstJitter        int     `json:"worst-jitter"`
	InterarrivalJitter int     `json:"interarrival-jitter"` // calculated with rfc3550 A.8 shortcut
}

type MTR struct {
	Done        chan struct{}
	OutputRaw   []byte
	Error       error
	PacketsSent int
	Hosts       []*Host `json:"hosts"`
}

// New runs mtr --raw -c reportCycles hostname args... Thus, you can add more arguments
// to the default required hostname and report cycles. The MTR call is signified as done
// when the MTR.Done chan closes. First wait for this, then check the MTR.Error field before
// looking at the output. Other than that, the fields and json tags document what everything
// means.
func New(reportCycles int, host string, args ...string) *MTR {
	m := &MTR{Done: make(chan struct{}), PacketsSent: reportCycles}
	args = append([]string{"--raw", "-c", strconv.Itoa(reportCycles), host}, args...)
	go func() {
		defer close(m.Done)
		m.OutputRaw, m.Error = exec.Command("mtr", args...).Output()
		if m.Error == nil {
			m.processOutput()
		}
	}()
	return m
}

func parseByteNum(input []byte) int {
	i := 0
	for _, v := range input {
		i = 10*i + int(v) - 48 // ascii 48 is `0`
	}
	return i
}

func parseHostnum(line []byte) (num int, finalFieldIdx int) {
	finalFieldIdx = bytes.IndexByte(line[2:], ' ') + 2
	return parseByteNum(line[2:finalFieldIdx]), finalFieldIdx + 1 // `c ### <content>`
}

func (m *MTR) processOutput() {
	// h (host): host #, ip address
	// d (dns): host #, resolved dns name
	// p (packet): host #, microseconds
	defer func() {
		if x := recover(); x != nil {
			m.Error = fmt.Errorf("Unable to process output, error %v, meaning the mtr output is WHACK! Check the output to see what's up!", x)
		}
	}()
	output := m.OutputRaw
	output = append(output, ' ') // tack on a space at the end so that `output = output[lineIdx+1:] doesn't panic on last newline
	for {
		lineIdx := bytes.IndexByte(output, '\n')
		if lineIdx == -1 {
			break
		}
		line := output[:lineIdx]
		output = output[lineIdx+1:]

		hostnum, finalFieldIdx := parseHostnum(line)

		switch line[0] {
		case 'h':
			for len(m.Hosts) < hostnum+1 {
				m.Hosts = append(m.Hosts, &Host{Hop: len(m.Hosts)})
			}
			m.Hosts[hostnum].IP = net.ParseIP(string(line[finalFieldIdx:]))
		case 'd':
			m.Hosts[hostnum].Name = string(line[finalFieldIdx:])
		case 'p':
			m.Hosts[hostnum].PacketMicrosecs = append(m.Hosts[hostnum].PacketMicrosecs, parseByteNum(line[finalFieldIdx:]))
		}
	}

	m.processHosts()
}

func (m *MTR) processHosts() {
	for _, host := range m.Hosts {
		host.Sent = m.PacketsSent
		host.Received = len(host.PacketMicrosecs)
		host.Dropped = host.Sent - host.Received
		host.LostPercent = float64(host.Dropped) / float64(host.Sent)
		if host.Received == 0 {
			continue
		}
		totalPacketTime := 0
		best := 1<<31 - 1
		worst := 0
		jitters := make([]int, host.Received)
		worstJitter := 0
		for i, packet := range host.PacketMicrosecs {
			if i > 0 {
				newJitter := packet - host.PacketMicrosecs[i-1]
				if newJitter < 0 {
					newJitter = -newJitter
				}
				if newJitter > worstJitter {
					worstJitter = newJitter
				}
				jitters[i] = newJitter
				host.InterarrivalJitter += newJitter - ((host.InterarrivalJitter + 8) >> 4) // rfc3550 A.8
			}
			totalPacketTime += packet
			if packet > worst {
				worst = packet
			}
			if packet < best {
				best = packet
			}
		}
		// mtr keeps a running average, so values may be different than a true average
		host.Mean = float64(totalPacketTime) / float64(host.Received)
		host.WorstJitter = worstJitter
		host.Best = best
		host.Worst = worst
		sqrDiff := float64(0)
		jitterSum := 0
		for i, packet := range host.PacketMicrosecs {
			diff := float64(packet) - host.Mean
			sqrDiff += diff * diff
			jitterSum += jitters[i]
		}
		host.MeanJitter = float64(jitterSum) / float64(host.Received)
		host.StandardDev = math.Sqrt(sqrDiff / float64(host.Mean))
	}
}
