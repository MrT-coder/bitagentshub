package daemon

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// DefaultAddr is this user's daemon pipe.
func DefaultAddr() string {
	sid, err := userSID()
	if err != nil {
		return `\\.\pipe\bah-daemon`
	}
	return `\\.\pipe\bah-` + sid
}

func userSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// Listen opens the pipe so only this user and SYSTEM can connect. A second
// daemon fails here, since the first one owns the pipe.
func Listen(addr string) (net.Listener, error) {
	sid, err := userSID()
	if err != nil {
		return nil, err
	}
	sddl := fmt.Sprintf("D:P(A;;GA;;;%s)(A;;GA;;;SY)", sid)
	return winio.ListenPipe(addr, &winio.PipeConfig{SecurityDescriptor: sddl})
}

func dialAddr(addr string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(addr, &timeout)
}

// spawn starts the daemon with no console and outside the caller's job, so
// neither closing the terminal nor ending an SSH session kills it. When the
// job forbids breaking away (Windows OpenSSH), the WMI service creates the
// process instead, which puts it outside the job.
func spawn(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB,
	}
	err := cmd.Start()
	if err == nil {
		return cmd.Process.Release()
	}
	line := syscall.EscapeArg(exe)
	for _, a := range args {
		line += " " + syscall.EscapeArg(a)
	}
	ps := fmt.Sprintf(`$r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=%s}; exit $r.ReturnValue`, psQuote(line))
	if out, err2 := exec.Command("powershell.exe", "-NoProfile", "-Command", ps).CombinedOutput(); err2 != nil {
		return fmt.Errorf("breakaway: %v; wmi: %v %s", err, err2, out)
	}
	return nil
}

func exeName(stem string) string { return stem + ".exe" }
