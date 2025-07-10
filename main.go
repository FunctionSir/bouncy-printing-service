/*
 * @Author: FunctionSir
 * @License: AGPLv3
 * @Date: 2025-07-10 20:18:43
 * @LastEditTime: 2025-07-10 23:41:17
 * @LastEditors: FunctionSir
 * @Description: -
 * @FilePath: /bouncy-printing-service/main.go
 */

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/FunctionSir/readini"
	"github.com/fsnotify/fsnotify"
)

const DEFAULT_GO_PRINT_CMD string = "!!!"
const DEFAULT_DEST string = ""
const DEFAULT_LP string = "lp"

// Do NOT update once set
var Options readini.Sec

var PrinterLock sync.Mutex

func Check(err error) {
	if err != nil {
		panic(err)
	}
}

func DirExists(path string) bool {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) || !stat.IsDir() {
		return false
	}
	return true
}

func loadConf() readini.Sec {
	if len(os.Args) < 2 {
		panic("no config file specified")
	}
	conf, err := readini.LoadFromFile(os.Args[1])
	Check(err)
	if !conf.HasSection("options") {
		panic("no section \"options\" found")
	}
	sec := conf["options"]
	if !sec.HasKey("ManagedDir") || sec["ManagedDir"] == "" {
		panic("no valid ManagedDir specified in config file")
	}
	if !sec.HasKey("GoPrintCmd") || sec["GoPrintCmd"] == "" {
		sec["GoPrintCmd"] = DEFAULT_GO_PRINT_CMD
	}
	if !sec.HasKey("Lp") || sec["Lp"] == "" {
		sec["Lp"] = DEFAULT_LP
	}
	if !sec.HasKey("Dest") || sec["Dest"] == "" {
		sec["Dest"] = DEFAULT_DEST
	}
	return sec
}

func doPrint(toPrint []string) {
	for _, file := range toPrint {
		log.Println("Start to print", file)
		lpArgs := make([]string, 0)
		lpArgs = append(lpArgs, file)
		if Options["Dest"] != "" {
			lpArgs = append(lpArgs, "-d", Options["Dest"])
		}
		log.Println("Will run command:", Options["Lp"], strings.Join(lpArgs, " "))
		cmd := exec.Command(Options["Lp"], lpArgs...)
		err := cmd.Run()
		if err != nil {
			log.Println("While printing", file, "error occurred:", err.Error())
			continue
		}
		log.Println("Command lp to print", file, "finished without error")
	}
}

func doTask(taskDir string) {
	PrinterLock.Lock()
	defer PrinterLock.Unlock()
	toPrint := make([]string, 0)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		log.Println("Can not read dir", taskDir, ":", err.Error())
	}
	for _, file := range entries {
		if file.Type().IsDir() {
			log.Println("In task", taskDir, "ignored dir", file.Name())
			continue
		}
		splited := strings.Split(file.Name(), ".")
		if splited[len(splited)-1] != "pdf" {
			log.Println("In task", taskDir, "ignored non-pdf file", file.Name())
			continue
		}
		info, err := file.Info()
		if err != nil {
			log.Println("In task", taskDir, "can not get file info of", file.Name(), "so ignored")
			continue
		}
		if info.Size() <= 0 {
			log.Println("In task", taskDir, "ignored", file.Name(), "because it seems empty")
			continue
		}
		toPrint = append(toPrint, path.Join(taskDir, file.Name()))
	}
	doPrint(toPrint)
	err = os.RemoveAll(taskDir)
	if err != nil {
		log.Println("In task", taskDir, "can not remove task dir after finished:", err.Error())
	}
	log.Println("Task", taskDir, "finished")
}

func main() {
	fmt.Println("Bouncy Printing Service [ Version: 0.1.0 ]")
	fmt.Println("By FunctionSir | This is a libre software under AGPLv3")
	Options = loadConf()
	if !DirExists(Options["ManagedDir"]) {
		err := os.Mkdir(Options["ManagedDir"], os.ModePerm)
		Check(err)
	}
	watcher, err := fsnotify.NewWatcher()
	Check(err)
	defer watcher.Close()
	err = watcher.Add(Options["ManagedDir"])
	Check(err)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Println("Got a not-ok event while watching fs changes")
				continue
			}
			if event.Op == fsnotify.Create && event.Name == path.Join(Options["ManagedDir"], Options["GoPrintCmd"]) {
				log.Println("Got valid GoPrint command")
				err := os.Remove(path.Join(Options["ManagedDir"], Options["GoPrintCmd"]))
				if err != nil {
					log.Println("Can not remove GoPrint command file:", err.Error())
					continue
				}
				thisTask := fmt.Sprintf("%s-%d", Options["ManagedDir"], time.Now().UnixNano())
				err = os.Rename(Options["ManagedDir"], thisTask)
				if err != nil {
					log.Println("Can not create a new task:", err.Error())
					continue
				}
				os.Mkdir(Options["ManagedDir"], os.ModePerm)
				log.Println("New task:", thisTask)
				go doTask(thisTask)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				log.Println("Got a not-ok event while watching fs changes")
				continue
			}
			log.Println("Got an error while watching fs changes: " + err.Error())
		}
	}
}
