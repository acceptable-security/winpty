package winpty

import (
	"bufio"
	"fmt"
	"testing"
)

func TestOpenClose(t *testing.T) {
	pty, err := NewWinPty(Coord{80, 25}, "dir")

	if err != nil {
		t.Errorf("Received unknown error: %s", err)
		return
	}

	pty.Close()
}


func TestResize(t *testing.T) {
	pty, err := NewWinPty(Coord{80, 25}, "dir")

	if err != nil {
		t.Errorf("Received unknown error: %s", err)
		return
	}

	pty.Resize(Coord{80, 30})
	pty.Close()
}

func TestRead(t *testing.T) {
	pty, err := NewWinPty(Coord{80, 25}, "ping -n 1 localhost")

	if err != nil {
		t.Errorf("Received unknown error: %s", err)
		return
	}

	go func() {
		scanner := bufio.NewScanner(pty)

		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	pty.Wait(1000)
	pty.Close()
}