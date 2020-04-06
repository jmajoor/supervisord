// +build windows

package signals

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	libkernel32                  = syscall.MustLoadDLL("kernel32")
	procSetConsoleCtrlHandler    = libkernel32.MustFindProc("SetConsoleCtrlHandler")
	procGenerateConsoleCtrlEvent = libkernel32.MustFindProc("GenerateConsoleCtrlEvent")
)

//convert a signal name to signal
func ToSignal(signalName string) (os.Signal, error) {
	if signalName == "HUP" {
		return syscall.SIGHUP, nil
	} else if signalName == "INT" {
		return syscall.SIGINT, nil
	} else if signalName == "QUIT" {
		return syscall.SIGQUIT, nil
	} else if signalName == "KILL" {
		return syscall.SIGKILL, nil
	} else if signalName == "USR1" {
		log.Warn("signal USR1 is not supported in windows")
		return nil, errors.New("signal USR1 is not supported in windows")
	} else if signalName == "USR2" {
		log.Warn("signal USR2 is not supported in windows")
		return nil, errors.New("signal USR2 is not supported in windows")
	} else if signalName == "BRK" {
		// Send USR1 (which is not defined in windows)
		return syscall.Signal(0xa), nil
	} else {
		return syscall.SIGTERM, nil

	}

}

//
// Args:
//    process - the process
//    sig - the signal
//    sigChildren - ignore in windows system
//
func Kill(process *os.Process, sig os.Signal, sigChilren bool) error {
	// Process generate console ctrl event on Signal.USR1
	if sig == syscall.Signal(0xa) {
		return generateConsoleCtrlEvent(process, sig, sigChilren)
	}
	//Signal command can't kill children processes, call  taskkill command to kill them
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", process.Pid))
	err := cmd.Start()
	if err == nil {
		return cmd.Wait()
	}
	//if fail to find taskkill, fallback to normal signal
	return process.Signal(sig)
}

func generateConsoleCtrlEvent(process *os.Process, sig os.Signal, sigChilren bool) error {
	// Make sure supervisord won't get the event.
	procSetConsoleCtrlHandler.Call(0, 1)
	// Restore the event listener for supervisord after we are done.
	defer procSetConsoleCtrlHandler.Call(0, 0)
	// Ideally we send an event to a console group (process.Pid) instead of 0.
	// You can create a console group in setDeathsig method by using:
	// sysProcAttr.CreationFlags = syscall.CREATE_UNICODE_ENVIRONMENT | 0x00000200
	// However for some reason passing a console group as the parameter to GenerateConsoleCtrlEvent inside
	// a container returns an error (Invalid function), so we use the brute force method and send the event
	// to all.
	r1, _, err := procGenerateConsoleCtrlEvent.Call(syscall.CTRL_C_EVENT, 0)

	if r1 == 0 {
		return err
	}

	// Since we can't send the event to a console group, we need to wait and make sure
	// the event was processed (otherwise the supervisord process receives the event and stops)
	// This event should only be called on a program that handles the event, so wait for it to exit.
	endTime := time.Now().Add(10 * time.Second)
	for endTime.After(time.Now()) {
		proc, _ := os.FindProcess(process.Pid)
		if proc == nil {
			break
		}
		proc.Release()
		time.Sleep(1 * time.Second)
	}
	return nil
}
