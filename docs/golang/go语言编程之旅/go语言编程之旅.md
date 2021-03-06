# 1. 命令行应用

### 1.1 子命令的使用

```go
package main

import (
	"flag"
	"log"
)

var name string

// go run main.go go -name=laowang
func main() {
	flag.Parse()

	goCmd := flag.NewFlagSet("go", flag.ExitOnError)
	goCmd.StringVar(&name, "name", "Go", "go help")
	phpCmd := flag.NewFlagSet("php", flag.ExitOnError)
	phpCmd.StringVar(&name, "n", "php", "php help")

	args := flag.Args()
	switch args[0] {
	case "go":
		_ = goCmd.Parse(args[1:])
	case "php":
		_ = phpCmd.Parse(args[1:])
	}
	log.Printf("name: %s", name)
}
```

### 1.2

使用 cobra 构建命令行 app： https://github.com/spf13/cobra

```go
// 使用 cobra 完成单词转换程序
package main

import (
	"log"
	"strings"

	"github.com/spf13/cobra"
)

const (
	MODE_UPPER = iota + 1
	MODE_LOWER
)

var wordCmd = &cobra.Command{
	Use:   "word",
	Short: "单词格式转换",
	Long:  "支持多种格式转换",
	Run: func(cmd *cobra.Command, args []string) {
		var content string
		switch mode {
		case MODE_UPPER:
			content = ToUpper(str)
		case MODE_LOWER:
			content = ToLower(str)
		default:
			log.Fatalf("暂不支持格式")
		}
		log.Printf("输出结果: %s", content)
	},
}

func ToUpper(s string) string {
	return strings.ToUpper(s)
}

func ToLower(s string) string {
	return strings.ToLower(s)
}

var desc = strings.Join([]string{
	"该命令支持单词转换，格式如下:",
	"1: 大写",
	"2: 小写",
}, "\n")

var str string // 输入
var mode int8

func init() {
	wordCmd.Flags().StringVarP(&str, "str", "s", "", "请输入单词内容")
	wordCmd.Flags().Int8VarP(&mode, "mode", "m", 0, "请输入单词转换模式")
}

// go run main.go word -s=laowang -m=1
func main() {
	err := wordCmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
```

### 1.3 时间工具

Local表示当前系统本地时区；UTC表示通用协调时间，也就是零时区。标准库time默认使用的是UTC时区。
2006-01-02 15：04：05是一个参考时间的格式，如同其他语言中的Y-m-d H：i：s格式，其功能是用于格式化处理时间。

### 1.4 SQL 语句到结构体转换

访问了MySQL数据库中的information_schema数据库，读取了COLUMNS表中对应的所需表的列信息，并基于标准库database/sql和text/template实现了表列信息到Go语言结构体的转换。


# 2. HTTP 应用

代码示例：https://github.com/go-programming-tour-book/blog-service.git

编译信息:

```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

/*
信号是UNIX、类UNIX，以及其他POSIX兼容的操作系统中进程间通信的一种有限制的方式。它是一种异步的通知机制，用来提醒进程一个事件（硬件异常、程序执行异常、外部发出信号）已经发生。当一个信号发送给一个进程时，操作系统中断了进程正常的控制流程。此时，任何非原子操作都将被中断。如果进程定义了信号的处理函数，那么它将被执行，否则执行默认的处理函数。

kill -l

ctrl + c  SIGINT  希望进程中断，进程结束
ctrl + z  SIGTSTP 任务中断，进程挂起
ctrl + \  SIGQUIT 进程结束和 dump core


kill -9 pid 发送 SIGKILL 信号给进程，强制中断进程
*/

func main() {
	// Set up channel on which to send signal notifications.
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	s := <-c
	fmt.Println("Got signal +++++:", s)
}
```


# 5 进程内缓存

https://github.com/go-programming-tour-book/cache-example

sync.Map类似于Go的map[interface{}]interface{}，但是它可以安全地被多个goroutine并发使用，而无须额外的锁。
在大部分情况下，我们都应该只使用普通的 map，在需要锁时再加上锁。sync.Map对以下两种场景进行了优化：

- 当给定key的条目仅被写入一次却被读取多次时，例如在仅增长的高速缓存中。
- 当多个goroutine读取、写入和覆盖不相交的键集合条目时。


# 6 Go 语言大杀器


讲了pprof, godebug, runtime/debug 等使用。


GOGC变量来调整初始垃圾收集器的目标百分比值，其规则为，当新分配的数值与上一次收集后剩余的实时数值的比例达到设置的目标百分比时，就会触发 GC。而GOGC的默认设置为GOGC=100，如果将其设置为GOGC=off，则可以完全禁用垃圾回收器。
一般来说，GOGC 的值设置得越大，GC 的频率越低，但每次 GC 所触发到的堆内存也会越大。在程序运行时，可以通过调用下述方法来动态调整GOGC的值：

```go
// runtime/debug
debug.SetGCPercent
```

go 进程诊断工具： gops (go process status)

