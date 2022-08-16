package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	stdaemon "github.com/takama/daemon"
	zdydaemon "github.com/zdypro888/daemon"
)

type file struct {
	URL  string `bson:"url" json:"url"`
	File string `bson:"file" json:"file"`
}
type update struct {
	Version int64   `bson:"ver" json:"ver"`
	Files   []*file `bson:"files" json:"files"`
}

type daemon struct {
	Name    string `bson:"name" json:"name"`
	Desc    string `bson:"desc" json:"desc"`
	URL     string `bson:"url" json:"url"`
	Version int64  `bson:"ver" json:"ver"`
}
type config struct {
	Daemons []*daemon `bson:"daemons" json:"daemons"`
}

func (con *config) Save() error {
	data, err := json.Marshal(con)
	if err != nil {
		return err
	}
	folder, err := osext.ExecutableFolder()
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(folder, "update.json"), data, 0644)
}
func (con *config) Load() error {
	folder, err := osext.ExecutableFolder()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path.Join(folder, "update.json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, con)
}

var defaultConfig = &config{}

func updateDaemon(upmon *daemon) bool {
	updateData, err := httpRequest(upmon.URL)
	if err != nil {
		log.Printf("Check daemon(%s) error: %v", upmon.Name, err)
		return false
	}
	updateInfo := &update{}
	if err := json.Unmarshal(updateData, updateInfo); err != nil {
		log.Printf("Check daemon(%s) error: %v", upmon.Name, err)
		return false
	}
	if upmon.Version >= updateInfo.Version {
		log.Printf("Check daemon(%s) no need update", upmon.Name)
		return false
	}

	daemonControl, err := stdaemon.New(upmon.Name, upmon.Desc, stdaemon.SystemDaemon)
	if err != nil {
		log.Printf("Control daemon(%s) error: %v", upmon.Name, err)
		return false
	}
	if str, err := daemonControl.Stop(); err != nil {
		log.Printf("Stop daemon(%s) error(%s): %v", upmon.Name, str, err)
	}
	time.Sleep(5 * time.Second)
	folder, err := osext.ExecutableFolder()
	if err != nil {
		log.Printf("Get executable folder error: %v", err)
		return false
	}
	for _, updateFile := range updateInfo.Files {
		fileData, err := httpRequest(updateFile.URL)
		if err != nil {
			log.Printf("Download daemon(%s) error: %v", upmon.Name, err)
			return false
		}
		dFilePath := updateFile.File
		if dFilePath[0] != '/' {
			dFilePath = path.Join(folder, dFilePath)
		}
		os.MkdirAll(path.Dir(dFilePath), os.ModePerm)
		if err := os.WriteFile(dFilePath, fileData, 0755); err != nil {
			log.Printf("Write daemon(%s) error: %v", upmon.Name, err)
			return false
		}
	}
	if str, err := daemonControl.Start(); err != nil {
		log.Printf("Start daemon(%s) error(%s): %v", upmon.Name, str, err)
	}
	upmon.Version = updateInfo.Version
	log.Printf("Update daemon(%s) Successful", upmon.Name)
	return true
}
func keepDaemon(d *daemon) bool {
	kpd, err := stdaemon.New(d.Name, d.Desc, stdaemon.SystemDaemon)
	if err != nil {
		log.Printf("Control daemon(%s) error: %v", d.Name, err)
		return false
	}
	var status string
	if status, err = kpd.Status(); err != nil {
		log.Printf("Get daemon(%s) Status error: %v", d.Name, err)
		return false
	}
	if strings.Contains(status, "running") {
		return true
	}
	if status, err = kpd.Start(); err != nil {
		log.Printf("Start daemon(%s) Status error: %v", d.Name, err)
		return false
	}
	log.Print(status)
	return true
}
func daemonUpdateGo() {
	for {
		for _, di := range defaultConfig.Daemons {
			if di.URL == "" || !updateDaemon(di) {
				keepDaemon(di)
			}
		}
		time.Sleep(1 * time.Minute)
	}
}

const (
	name        = "updater"
	description = "auto keep&update daemon service"
)

var dependencies = []string{}

func main() {
	if !zdydaemon.RunWithConsole(name, description, dependencies...) {
		return
	}
	if err := defaultConfig.Load(); err != nil {
		defaultConfig.Daemons = append(defaultConfig.Daemons, &daemon{
			Name:    "daemon",
			Desc:    "daemon desc",
			URL:     "http://update.com/update.json",
			Version: 20210101,
		})
		defaultConfig.Save()
		log.Printf("Read config file faild %v", err)
		return
	}

	defer defaultConfig.Save()
	go daemonUpdateGo()

	interrupt := make(chan os.Signal, 1)
	defer close(interrupt)
	signal.Notify(interrupt, os.Interrupt, os.Kill, syscall.SIGTERM)
	killSignal := <-interrupt
	if killSignal == os.Interrupt {
		log.Printf("Interruped by system signal")
	}
}
