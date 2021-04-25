package lib

import "github.com/chzyer/readline"

func createReadLine() *readline.Instance {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "> ",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,
		FuncFilterInputRune: func(r rune) (rune, bool) {
			switch r {
			case readline.CharCtrlZ:
				return r, false
			}
			return r, true
		},
	})
	if err != nil {
		panic(err)
	}

	return rl
}

var instance *readline.Instance

func ReadLine() *readline.Instance {
	if instance == nil {
		instance = createReadLine()
	}

	return instance
}