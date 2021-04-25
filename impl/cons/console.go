package cons

import (
	"github.com/chzyer/readline"
	"github.com/golangmc/minecraft-server/apis/uuid"
	"github.com/golangmc/minecraft-server/lib"
	"io"
	"os"

	"github.com/golangmc/minecraft-server/apis/base"
	"github.com/golangmc/minecraft-server/apis/logs"
	"github.com/golangmc/minecraft-server/impl/data/system"
)

type Console struct {
	o io.Writer

	rl *readline.Instance

	logger *logs.Logging

	IChannel chan string
	OChannel chan string

	report chan system.Message
}

func NewConsole(report chan system.Message) *Console {
	console := &Console{
		IChannel: make(chan string),
		OChannel: make(chan string),

		report: report,
	}

	console.rl = lib.ReadLine()
	console.o = io.MultiWriter(console.rl.Stdout(), console.newLogFile("latest.log"))
	console.logger = logs.NewLoggingWith("console", console.o, logs.EveryLevel...)

	return console
}

func (c *Console) Load() {
	// handle i channel
	go func() {
		for {
			line, err := c.rl.Readline()

			if err == readline.ErrInterrupt && len(line) != 0 {
				continue
			}

			if err == readline.ErrInterrupt || err == io.EOF {
				c.report <- system.Make(system.STOP, "normal stop")
				return
			}

			err = base.Attempt(func() {
				c.IChannel <- line
			})

			if err != nil {
				c.report <- system.Make(system.FAIL, err)
			}
		}
	}()

	// handle o channel
	go func() {
		for line := range c.OChannel {
			c.logger.Info(line)
		}
	}()

	go func() {
		select {
		case command := <-c.report:
			switch command.Command {
			// stop selecting when stop is received
			case system.STOP:
				return
			case system.FAIL:
				return
			}
		}
	}()
}

func (c *Console) Kill() {
	defer func() {
		_ = recover() // ignore panic with closing closed channel
	}()

	// save the log file as YYYY-MM-DD-{index}.log{.gz optionally compressed}

	close(c.OChannel)
}

func (c *Console) Name() string {
	return "ConsoleSender"
}

func (c *Console) SendMessage(message ...interface{}) {
	defer func() {
		if err := recover(); err != nil {
			c.report <- system.Make(system.FAIL, err)
		}
	}()

	c.OChannel <- base.ConvertToString(message...)
}

type logFileWriter struct {
	file *os.File
}

func (c *Console) newLogFile(name string) io.Writer {
	file, err := os.Create(name)
	if err != nil {
		c.report <- system.Make(system.FAIL, err)
		return nil
	}

	return &logFileWriter{file: file}
}

func (l *logFileWriter) Write(p []byte) (n int, err error) {

	// this is going to be messy, but this should convert to string, strip colors, and then write to file. Don't @ me.

	return l.file.Write(p)
}

func (c *Console) UUID() uuid.UUID {
	return uuid.TextToUUID(c.Name())
}
