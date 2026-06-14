package output

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

func PrintLinesTail(path string, n int, out io.Writer) error {
	if n < 0 {
		n = 80
	}
	if n == 0 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			copy(lines, lines[1:])
			lines = lines[:n]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}

func FollowFile(path string, out io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			fmt.Fprint(out, line)
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return err
		}
		if _, err := file.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
		SleepBriefly()
	}
}

func SleepBriefly() {
	time.Sleep(500 * time.Millisecond)
}
