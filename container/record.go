package container

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"xwj/mydocker/log"
	"xwj/mydocker/record"
	"xwj/mydocker/utils"
)

var (
	RUNNING             = "running"
	STOP                = "stopped"
	EXIT                = "exited"
	DefaultInfoLocation = "/var/run/mydocker/"
	ConfigName          = "containerInfo.json"
	LogFileName         = "container.log"
)

// RandStringContainerID 容器ID随机生成器
func RandStringContainerID(n int) string {
	if n < 0 || n > 32 {
		n = 32
	}
	// 这里就采用对时间戳取hash的方法实现容器的随机ID生成
	hashBytes := sha256.Sum256([]byte(strconv.Itoa(int(time.Now().UnixNano()))))
	return fmt.Sprintf("%x", hashBytes[:n])
}

// RecordContainerInfo 记录一个容器的信息
func RecordContainerInfo(id string, cPID int, commandArray []string, cName, volume string, port []string) (*record.ContainerInfo, error) {
	// 以当前时间为容器的创建时间
	createTime := time.Now().Format("2006-01-02 15:04:05")
	// 如果用户没有指定容器名就用容器ID做为容器名
	if cName == "" {
		cName = id
	}
	containerInfo := record.ContainerInfo{
		Pid:         strconv.Itoa(cPID),
		Id:          id,
		Name:        cName,
		Command:     strings.Join(commandArray, ""),
		Volume:      volume,
		CreatedTime: createTime,
		Status:      RUNNING,
		PortMapping: port,
	}
	// 序列为json
	jsonBytes, err := json.Marshal(containerInfo)
	if err != nil {
		log.LogErrorFrom("recordContainerInfo", "Marshal", err)
		return nil, err
	}
	// 创建容器信息对应的文件夹
	dirUrl := filepath.Join(DefaultInfoLocation, id)
	if err := os.MkdirAll(dirUrl, 0622); err != nil {
		log.LogErrorFrom("recordContainerInfo", "MkdirAll", err)
		return nil, err
	}
	// 创建json文件
	fileName := filepath.Join(dirUrl, ConfigName)
	configFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		log.LogErrorFrom("recordContainerInfo", "OpenFile", err)
		return nil, err
	}
	defer configFile.Close()
	// 写入到文件
	if _, err := configFile.WriteString(string(jsonBytes)); err != nil {
		log.LogErrorFrom("recordContainerInfo", "WriteString", err)
		return nil, err
	}
	return &containerInfo, nil
}

// recordContainerLog 创建容器进程的日志文件并将其标准输出重定向到此文件
func recordContainerLog(id string, cmdOut *io.Writer) {
	dirUrl := filepath.Join(DefaultInfoLocation, id)
	if has, err := utils.DirOrFileExist(dirUrl); err == nil && !has {
		if err := os.MkdirAll(dirUrl, 0622); err != nil {
			log.LogErrorFrom("recordContainerLog", "MkdirAll", err)
			return
		}
	}
	stdLogFilePath := filepath.Join(dirUrl, LogFileName)
	stdLogFile, err := os.Create(stdLogFilePath)
	if err != nil {
		log.LogErrorFrom("recordContainerLog", "Create", err)
		return
	}
	*cmdOut = stdLogFile
}

// DeleteContainerInfo 删除一个容器的容器ID
func DeleteContainerInfo(containerID string) {
	dirUrl := filepath.Join(DefaultInfoLocation, containerID)
	if err := os.RemoveAll(dirUrl); err != nil {
		log.LogErrorFrom("DeleteContainerInfo", "RemoveAll", err)
	}
}

// ListAllContainers  列出所有容器信息，输出到标准输出
func ListAllContainers() {
	dirUrl := filepath.Join(DefaultInfoLocation)
	// 读取该路径下的所有文件
	files, err := ioutil.ReadDir(dirUrl)
	if err != nil {
		log.LogErrorFrom("ListAllContainers", "ReadDir", err)
		return
	}
	var containers []*record.ContainerInfo
	for _, file := range files {
		// 排除掉network文件夹的影响
		if file.Name() == "network" {
			continue
		}
		tmpContainerInfo, err := getContainerInfo(file)
		if err != nil {
			log.LogErrorFrom("ListAllContainers", "getContainerInfo", err)
			continue
		}
		containers = append(containers, tmpContainerInfo)
	}
	// 输出
	// 使用tabwriter.NewWriter在控制台打印对齐的表格
	w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
	// 控制台输出的信息列
	fmt.Fprint(w, "ID\tNAME\tPID\tSTATUS\tCOMMAND\tCREATED\n")
	for _, item := range containers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t\n",
			item.Id,
			item.Name,
			item.Pid,
			item.Status,
			item.Command,
			item.CreatedTime,
		)
	}
	// 刷新标准输出刘缓冲区，将容器列表打印出来
	if err := w.Flush(); err != nil {
		log.LogErrorFrom("ListAllContainers", "Flush", err)
		return
	}
}

// getContainerInfo 获取一个容器的信息
func getContainerInfo(file os.FileInfo) (*record.ContainerInfo, error) {
	// 获取文件名称
	fileName := file.Name()
	// 生成文件的绝对路径
	filePath := filepath.Join(DefaultInfoLocation, fileName, ConfigName)
	// 读取文件信息
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.LogErrorFrom("getContainerInfo", "ReadFile", err)
		return nil, err
	}
	var containerInfo record.ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.LogErrorFrom("getContainerInfo", "Unmarshal", err)
		return nil, err
	}
	return &containerInfo, nil
}

// LogContainer 输出一个容器的日志
func LogContainer(containerId string) {
	logFilePath := filepath.Join(DefaultInfoLocation, containerId, LogFileName)
	file, err := os.OpenFile(logFilePath, os.O_RDONLY, 0644)
	if err != nil {
		log.LogErrorFrom("LogContainer", "OpenFile", err)
		return
	}
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.LogErrorFrom("LogContainer", "ReadAll", err)
		return
	}
	// 使用Fprint函数将读出来的文件内容输出到宿主机的标准输出/控制台中
	_, err = fmt.Fprint(os.Stdout, string(content))
	if err != nil {
		log.LogErrorFrom("LogContainer", "Fprint", err)
		return
	}
}
