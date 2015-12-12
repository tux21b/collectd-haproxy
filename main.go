// Copyright (c) 2015 by Christoph Hack <christoph@tux21b.org>
// All rights reserved. Distributed under the Simplified BSD License.

// Command collectd-haproxy implements a collectd exec plugin to collect
// metrics from haproxy via an admin socket.
//
// Example configuration:
//   LoadPlugin exec
//   <Plugin exec>
//     Exec "haproxy" "/path/to/collectd-haproxy" "-silent"
//   </Plugin>
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"collectd.org/api"
	"collectd.org/exec"
)

var (
	hostname string
	plugin   string
	socket   string
	interval time.Duration
)

func main() {
	var (
		flagSocket = flag.String("socket", "/var/run/haproxy/admin.sock", "haproxy admin socket")
		flagPlugin = flag.String("plugin", "haproxy", "plugin name")
		flagSilent = flag.Bool("silent", false, "silent mode")
	)
	if flag.Parse(); flag.NArg() != 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *flagSilent {
		log.SetOutput(ioutil.Discard)
	}

	socket = *flagSocket
	plugin = *flagPlugin
	hostname = exec.Hostname()
	interval = exec.Interval()

	e := exec.NewExecutor()
	e.VoidCallback(collectInfo, interval)
	e.VoidCallback(collectStats, interval)
	e.Run()
}

func collectInfo(interval time.Duration) {
	now := time.Now()
	buf := newBuffer()
	defer freeBuffer(buf)

	if err := communicate("show info", buf); err != nil {
		log.Println("communicate:", err)
		return
	}
	for {
		line, err := buf.ReadBytes('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			log.Println("readbytes:", line, err)
			break
		}
		i := bytes.IndexByte(line, ':')
		if i < 0 || i+1 >= len(line) {
			continue
		}
		key := string(bytes.ToLower(bytes.TrimSpace(line[:i])))
		value := string(bytes.TrimSpace(line[i+1:]))
		reportMetric(key, "", "", value, now, interval)
	}
}

func collectStats(interval time.Duration) {
	now := time.Now()
	buf := newBuffer()
	defer freeBuffer(buf)

	if err := communicate("show stat", buf); err != nil {
		log.Println("communicate:", err)
		return
	}

	r := csv.NewReader(buf)
	header, err := r.Read()
	if err != nil {
		log.Println("failed to read header:", err)
		return
	}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if len(record) < 2 {
			continue
		}
		pxname, svname := record[0], record[1]
		for i := 2; i < len(header) && i < len(record); i++ {
			reportMetric(header[i], pxname, svname, record[i], now, interval)
		}
	}
}

func reportMetric(key, pxname, svname, value string, now time.Time, interval time.Duration) {
	key = strings.ToLower(strings.TrimSpace(key))
	m, ok := metricTypes[key]
	if !ok {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	pxname = strings.ToLower(strings.TrimSpace(pxname))
	svname = strings.ToLower(strings.TrimSpace(svname))

	name := m.TypeInstance
	if svname != "" || pxname != "" {
		name = strings.Join([]string{svname, pxname, name}, ".")
	}

	vl := api.ValueList{
		Identifier: api.Identifier{
			Host:         hostname,
			Plugin:       plugin,
			Type:         m.Type,
			TypeInstance: name,
		},
		Time:     now,
		Interval: interval,
	}
	switch m.Type {
	case "derive":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			log.Printf("can not convert value for %q: %v\n", key, err)
			return
		}
		vl.Values = []api.Value{api.Derive(v)}
	case "gauge":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			log.Printf("can not convert value for %q: %v\n", key, err)
			return
		}
		vl.Values = []api.Value{api.Gauge(v)}
	default:
		log.Println("invalid type", m.Type)
		return
	}
	exec.Putval.Write(vl)
}

func communicate(command string, buf *bytes.Buffer) error {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := fmt.Fprintln(conn, command); err != nil {
		return err
	}
	buf.Reset()
	if _, err := io.Copy(buf, conn); err != nil {
		return err
	}
	return nil
}

var pool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func newBuffer() *bytes.Buffer {
	return pool.Get().(*bytes.Buffer)
}

func freeBuffer(buf *bytes.Buffer) {
	buf.Reset()
	pool.Put(buf)
}

var metricTypes = map[string]struct {
	TypeInstance string
	Type         string
}{
	"maxconn":      {"max_connections", "gauge"},
	"cumconns":     {"connections", "derive"},
	"cumreq":       {"requests", "derive"},
	"currconns":    {"cur_connections", "gauge"},
	"currsslconns": {"cur_ssl_connections", "gauge"},
	"maxconnrate":  {"max_connection_rate", "gauge"},
	"maxsessrate":  {"max_session_rate", "gauge"},
	"maxsslconns":  {"max_ssl_connections", "gauge"},
	"cumsslconns":  {"ssl_connections", "derive"},
	"maxpipes":     {"max_pipes", "gauge"},
	"idle_pct":     {"idle_pct", "gauge"},
	"tasks":        {"tasks", "gauge"},
	"run_queue":    {"run_queue", "gauge"},
	"pipesused":    {"pipes_used", "gauge"},
	"pipesfree":    {"pipes_free", "gauge"},
	"uptime_sec":   {"uptime_seconds", "derive"},
	"bin":          {"bytes_in", "derive"},
	"bout":         {"bytes_out", "derive"},
	"chkfail":      {"failed_checks", "derive"},
	"downtime":     {"downtime", "derive"},
	"dresp":        {"denied_response", "derive"},
	"dreq":         {"denied_request", "derive"},
	"econ":         {"error_connection", "derive"},
	"ereq":         {"error_request", "derive"},
	"eresp":        {"error_response", "derive"},
	"hrsp_1xx":     {"response_1xx", "derive"},
	"hrsp_2xx":     {"response_2xx", "derive"},
	"hrsp_3xx":     {"response_3xx", "derive"},
	"hrsp_4xx":     {"response_4xx", "derive"},
	"hrsp_5xx":     {"response_5xx", "derive"},
	"hrsp_other":   {"response_other", "derive"},
	"qcur":         {"queue_current", "gauge"},
	"rate":         {"session_rate", "gauge"},
	"req_rate":     {"request_rate", "gauge"},
	"stot":         {"session_total", "derive"},
	"scur":         {"session_current", "gauge"},
	"wredis":       {"redistributed", "derive"},
	"wretr":        {"retries", "derive"},
}
