package winpty

/*
#define NTDDI_VERSION 0x0A000007
#define WINVER 0x0A00

#include <SDKDDKVer.h>
#include <Windows.h>
#include <wincon.h>

#undef NTDDI_VERSION
#undef WINVER

// Stolen from https://docs.microsoft.com/en-us/windows/console/closepseudoconsole
HRESULT PrepareStartupInformation(HPCON hpc, STARTUPINFOEX* psi)
{
    // Prepare Startup Information structure
    STARTUPINFOEX si;
    ZeroMemory(&si, sizeof(si));
    si.StartupInfo.cb = sizeof(STARTUPINFOEX);

    // Discover the size required for the list
    size_t bytesRequired;
    InitializeProcThreadAttributeList(NULL, 1, 0, &bytesRequired);
    
    // Allocate memory to represent the list
    si.lpAttributeList = (PPROC_THREAD_ATTRIBUTE_LIST)HeapAlloc(GetProcessHeap(), 0, bytesRequired);
    if (!si.lpAttributeList)
    {
        return E_OUTOFMEMORY;
    }

    // Initialize the list memory location
    if (!InitializeProcThreadAttributeList(si.lpAttributeList, 1, 0, &bytesRequired))
    {
        HeapFree(GetProcessHeap(), 0, si.lpAttributeList);
        return HRESULT_FROM_WIN32(GetLastError());
    }

    // Set the pseudoconsole information into the list
    if (!UpdateProcThreadAttribute(si.lpAttributeList,
                                   0,
                                   PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
                                   hpc,
                                   sizeof(hpc),
                                   NULL,
                                   NULL))
    {
        HeapFree(GetProcessHeap(), 0, si.lpAttributeList);
        return HRESULT_FROM_WIN32(GetLastError());
    }

    *psi = si;

    return S_OK;
}

*/
import "C"

import (
	"fmt"
	"syscall"
)

// A 16-bit 2D point.
type Coord struct {
	X uint16
	Y uint16
}

// Container for the associated pseudo console
type WinPty struct {
	processInfo C.PROCESS_INFORMATION
	processStartup C.STARTUPINFOEX
	handle C.HPCON // Windows API handle to the pseudo console
	inputPipe C.HANDLE // Input pipes of pseudo console
	outputPipe C.HANDLE // Output pipes of pseudo console
}

// Allocate a new pseudo console of a given size.
func NewWinPty(size Coord, cmd string) (pty *WinPty, err error) {
	pty = new(WinPty)

	if err = pty.createPty(size); err != nil {
		return nil, err
	}

	if err = pty.createStartupInfo(); err != nil {
		return nil, err
	}

	if err = pty.spawn(cmd); err != nil {
		return nil, err
	}

	return pty, nil
}

// Allocate pseudo console
func (pty *WinPty) createPty(size Coord) (err error) {
	var pipePTYIn C.HANDLE
	var pipePTYOut C.HANDLE

	if C.CreatePipe(&pipePTYIn, &pty.outputPipe, nil, 0) == 0 || C.CreatePipe(&pty.inputPipe, &pipePTYOut, nil, 0) == 0 {
		return fmt.Errorf("Failed to create pipes.")
	}

	// Convert golang coord to C coord
	var coord C.COORD
	coord.X = C.short(size.X)
	coord.Y = C.short(size.Y)

	err2 := C.CreatePseudoConsole(coord, pipePTYIn, pipePTYOut, 0, &pty.handle)

	if C.HRESULT(err2) < 0 {
		// TODO - Process err2 correctly.
		return fmt.Errorf("Unable to call CreatePseudoConsole")
	}

	C.CloseHandle(pipePTYIn)
	C.CloseHandle(pipePTYOut)

	return nil
}

// Allocate startup info for subprocess
func (pty *WinPty) createStartupInfo() (err error) {
	_err := C.PrepareStartupInformation(pty.handle, &pty.processStartup)

	if C.HRESULT(_err) < 0 {
		// TODO - Process err2 correctly.
		return fmt.Errorf("Unable to call PrepareStartupInformation")
	}

	return nil
}

// Spawn the subprocess
func (pty *WinPty) spawn(cmd string) (err error) {
	cCmd := C.CString(cmd)

	// Create subprocess with pty
	_err := C.CreateProcess(
		nil,
		cCmd,
		nil,
		nil,
		C.FALSE,
		C.EXTENDED_STARTUPINFO_PRESENT,
		nil,
		nil,
		&pty.processStartup.StartupInfo,
		&pty.processInfo,
	)

	if C.HRESULT(_err) < 0 {
		// TODO - Process err2 correctly.
		return fmt.Errorf("Unable to call CreateProcessW")
	}

	return nil
}

// Close the pipehandles used during the lifetime of the pty
func (pty *WinPty) closePipeHandles() {
	C.CloseHandle(pty.inputPipe)
	C.CloseHandle(pty.outputPipe)
}

// Close the thread and processe and associated memory
func (pty *WinPty) closeProcessHandle() {
	syscall.Close(syscall.Handle(pty.processInfo.hThread))
	syscall.Close(syscall.Handle(pty.processInfo.hProcess))

	C.DeleteProcThreadAttributeList(pty.processStartup.lpAttributeList)
	C.HeapFree(C.GetProcessHeap(), 0, C.LPVOID(pty.processStartup.lpAttributeList))
}

// Resize the pseduo console to a new size
func (pty *WinPty) Resize(size Coord) {
	var coord C.COORD
	coord.X = C.short(size.X)
	coord.Y = C.short(size.Y)

	C.ResizePseudoConsole(pty.handle, coord)
}

// Read n bytes from the console
func (pty *WinPty) Read(p []byte) (n int, err error) {
	var out C.ulong
	
	// Create an equal sized buffer to copy into
	buf := make([]byte, len(p))
	cBuf := C.CBytes(buf)

	// Do the read
	err2 := C.ReadFile(pty.inputPipe, C.LPVOID(cBuf), C.ulong(len(p)), &out, nil)

	if err2 == 0 {
		C.free(cBuf)

		if C.GetLastError() == C.ERROR_IO_PENDING {
			return int(out), nil
		}

		return 0, fmt.Errorf("Unable to call ReadFile: %x", C.GetLastError())
	}

	// Copy into output buffer
	x := C.int(len(p))
	done := C.GoBytes(cBuf, x)

	for i := 0; i < len(p); i++ {
		p[i] = done[i]
	}

	return int(out), nil
}

// Write n bytes to the console
func (pty *WinPty) Write(p []byte) (n int, err error) {
	var out C.ulong
	err2 := C.WriteFile(pty.outputPipe, C.LPCVOID(C.CBytes(p)), C.ulong(len(p)), &out, nil)

	if err2 == 0 {
		if C.GetLastError() == C.ERROR_IO_PENDING {
			return int(out), nil
		}

		return 0, fmt.Errorf("Unable to call WriteFile: %x", C.GetLastError())
	}

	return int(out), nil
}

// Wait for the program to terminate
func (pty *WinPty) Wait(time int) {
	C.WaitForSingleObject(pty.processInfo.hThread, C.ulong(time))
}

// Close and deallocate the pseudo console
func (pty *WinPty) Close() {
	pty.closeProcessHandle()
	pty.closePipeHandles()
	C.ClosePseudoConsole(pty.handle)
}