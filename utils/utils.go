// Statping
// Copyright (C) 2018.  Hunter Long and the project contributors
// Written by Hunter Long <info@socialeck.com> and the project contributors
//
// https://github.com/statping/statping
//
// The licenses for most software and other practical works are designed
// to take away your freedom to share and change the works.  By contrast,
// the GNU General Public License is intended to guarantee your freedom to
// share and change all versions of a program--to make sure it remains free
// software for all its users.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package utils

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/ararog/timeago"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	// Directory returns the current path or the STATPING_DIR environment variable
	Directory   string
	disableLogs bool
)

// init will set the utils.Directory to the current running directory, or STATPING_DIR if it is set
func init() {
	defaultDir, err := os.Getwd()
	if err != nil {
		defaultDir = "."
	}

	Directory = Getenv("STATPING_DIR", defaultDir).(string)

	// check if logs are disabled
	disableLogs = Getenv("DISABLE_LOGS", false).(bool)
	if disableLogs {
		Log.Out = ioutil.Discard
	}

	Log.Debugln("current working directory: ", Directory)
	Log.AddHook(new(hook))
	Log.SetNoLock()
	checkVerboseMode()
}

func Getenv(key string, defaultValue interface{}) interface{} {
	if val, ok := os.LookupEnv(key); ok {
		if val != "" {
			switch d := defaultValue.(type) {

			case int, int64:
				return int(ToInt(val))

			case time.Duration:
				dur, err := time.ParseDuration(val)
				if err != nil {
					return d
				}
				return dur
			case bool:
				ok, err := strconv.ParseBool(val)
				if err != nil {
					return d
				}
				return ok
			default:
				return val
			}
		}
	}
	return defaultValue
}

// ToInt converts a int to a string
func ToInt(s interface{}) int64 {
	switch v := s.(type) {
	case string:
		val, _ := strconv.Atoi(v)
		return int64(val)
	case []byte:
		val, _ := strconv.Atoi(string(v))
		return int64(val)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	default:
		return 0
	}
}

// ToString converts a int to a string
func ToString(s interface{}) string {
	switch v := s.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", v)
	case float32, float64:
		return fmt.Sprintf("%f", v)
	case []byte:
		return string(v)
	case bool:
		return fmt.Sprintf("%t", v)
	case time.Time:
		return v.Format("Monday January _2, 2006 at 03:04PM")
	case time.Duration:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

type Timestamp time.Time
type Timestamper interface {
	Ago() string
}

// Ago returns a human readable timestamp based on the Timestamp (time.Time) interface
func (t Timestamp) Ago() string {
	got, _ := timeago.TimeAgoWithTime(time.Now(), time.Time(t))
	return got
}

// Command will run a terminal command with 'sh -c COMMAND' and return stdout and errOut as strings
//		in, out, err := Command("sass assets/scss assets/css/base.css")
func Command(name string, args ...string) (string, string, error) {
	Log.Debugln("running command: " + name + strings.Join(args, " "))
	testCmd := exec.Command(name, args...)
	var stdout, stderr []byte
	var errStdout, errStderr error
	stdoutIn, _ := testCmd.StdoutPipe()
	stderrIn, _ := testCmd.StderrPipe()
	err := testCmd.Start()
	if err != nil {
		return "", "", err
	}

	go func() {
		stdout, errStdout = copyAndCapture(os.Stdout, stdoutIn)
	}()

	go func() {
		stderr, errStderr = copyAndCapture(os.Stderr, stderrIn)
	}()

	err = testCmd.Wait()
	if err != nil {
		return string(stdout), string(stderr), err
	}

	if errStdout != nil || errStderr != nil {
		return string(stdout), string(stderr), errors.New("failed to capture stdout or stderr")
	}

	outStr, errStr := string(stdout), string(stderr)
	return outStr, errStr, err
}

// copyAndCapture will read a terminal command into bytes
func copyAndCapture(w io.Writer, r io.Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1024, 1024)
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			d := buf[:n]
			out = append(out, d...)
			_, err := w.Write(d)
			if err != nil {
				return out, err
			}
		}
		if err != nil {
			// Read returns io.EOF at the end of file, which is not an error for us
			if err == io.EOF {
				err = nil
			}
			return out, err
		}
	}
}

// DurationReadable will return a time.Duration into a human readable string
// // t := time.Duration(5 * time.Minute)
// // DurationReadable(t)
// // returns: 5 minutes
func DurationReadable(d time.Duration) string {
	if d.Hours() >= 1 {
		return fmt.Sprintf("%0.0f hours", d.Hours())
	} else if d.Minutes() >= 1 {
		return fmt.Sprintf("%0.0f minutes", d.Minutes())
	} else if d.Seconds() >= 1 {
		return fmt.Sprintf("%0.0f seconds", d.Seconds())
	}
	return d.String()
}

// HttpRequest is a global function to send a HTTP request
// // url - The URL for HTTP request
// // method - GET, POST, DELETE, PATCH
// // content - The HTTP request content type (text/plain, application/json, or nil)
// // headers - An array of Headers to be sent (KEY=VALUE) []string{"Authentication=12345", ...}
// // body - The body or form data to send with HTTP request
// // timeout - Specific duration to timeout on. time.Duration(30 * time.Seconds)
// // You can use a HTTP Proxy if you HTTP_PROXY environment variable
func HttpRequest(url, method string, content interface{}, headers []string, body io.Reader, timeout time.Duration, verifySSL bool) ([]byte, *http.Response, error) {
	var err error
	var req *http.Request
	t1 := time.Now()
	if req, err = http.NewRequest(method, url, body); err != nil {
		httpMetric.Errors++
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Statping")
	if content != nil {
		req.Header.Set("Content-Type", content.(string))
	}

	verifyHost := req.URL.Hostname()
	for _, h := range headers {
		keyVal := strings.SplitN(h, "=", 2)
		if len(keyVal) == 2 {
			if keyVal[0] != "" && keyVal[1] != "" {
				if strings.ToLower(keyVal[0]) == "host" {
					req.Host = strings.TrimSpace(keyVal[1])
					verifyHost = req.Host
				} else {
					req.Header.Set(keyVal[0], keyVal[1])
				}
			}
		}
	}
	var resp *http.Response

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: timeout,
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
			ServerName:         verifyHost,
		},
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   timeout,
		Proxy:                 http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// redirect all connections to host specified in url
			addr = strings.Split(req.URL.Host, ":")[0] + addr[strings.LastIndex(addr, ":"):]
			return dialer.DialContext(ctx, network, addr)
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	if resp, err = client.Do(req); err != nil {
		httpMetric.Errors++
		return nil, resp, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)

	// record HTTP metrics
	t2 := time.Now().Sub(t1).Milliseconds()
	httpMetric.Requests++
	httpMetric.Milliseconds += t2 / httpMetric.Requests
	httpMetric.Bytes += int64(len(contents))

	return contents, resp, err
}
