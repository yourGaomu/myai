package main

import (
	"fmt"
	"os"

	"myai/core/cmd"
)

func main() {
	// main 只负责把控制权交给 Cobra 命令树；具体启动 Chat、Agent 或 Relay 的逻辑都在 core/cmd。
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
