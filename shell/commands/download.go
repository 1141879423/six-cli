package commands

import (
	"fmt"
	httpDownloader "github.com/Mrs4s/go-http-downloader"
	pl "github.com/Mrs4s/power-liner"
	"github.com/Mrs4s/six-cli/models"
	"github.com/Mrs4s/six-cli/shell"
	"github.com/Mrs4s/six-cli/six_cloud"
	"github.com/cheggaaa/pb"
	"strings"
	"time"
)

func init() {
	alias["Download"] = []string{"down"}
	explains["Download"] = "下载 文件夹/文件"
}

func (CommandHandler) Download(c *pl.Context) {
	if len(c.Nokeys) == 0 {
		fmt.Println("[H] 使用方法: down <文件/目录>")
		return
	}
	path := c.Nokeys[0]
	targetPath := shell.CurrentPath
	if len(path) == 0 {
		fmt.Println("[H] 使用方法: down <文件/目录>")
		return
	}
	if path[0:1] == "/" {
		targetPath = models.GetParentPath(path)
	}
	files, err := shell.CurrentUser.GetFilesByPath(targetPath)
	if err != nil {
		fmt.Println("[!] 错误:", err)
		return
	}
	var target *six_cloud.SixFile
	for _, file := range files {
		if file.Name == models.GetFileName(path) {
			target = file
			continue
		}
	}
	if target == nil {
		fmt.Println("[!] 错误: 目标文件/目录不存在")
		return
	}
	var downloaders []*httpDownloader.DownloaderClient
	for key, file := range target.GetLocalTree(models.DefaultConf.DownloadPath) {
		fmt.Println("[+] 添加下载", models.ShortString(models.GetFileName(file.Path), 70))
		addr, err := file.GetDownloadAddress()
		if err != nil {
			fmt.Println("[!] 获取文件", file.Name, "的下载链接失败:", err)
			continue
		}
		info, err := httpDownloader.NewDownloaderInfo([]string{addr}, key, models.DefaultConf.DownloadBlockSize, int(models.DefaultConf.DownloadThread),
			map[string]string{"User-Agent": "Six-cli download engine"})
		downloaders = append(downloaders, httpDownloader.NewClient(info))
	}
	ch := make(chan bool)
	defer close(ch)
	var bars []*pb.ProgressBar
	for _, task := range downloaders {
		bar := pb.New64(task.Info.ContentSize).Prefix(models.ShortString(models.GetFileName(task.Info.TargetFile), 20)).SetUnits(pb.U_BYTES)
		bar.ShowSpeed = true
		bars = append(bars, bar)
	}
	pool, err := pb.StartPool(bars...)
	if err != nil {
		fmt.Println("[!] 创建进度条失败, 无法显示进度，请等待后台下载完成.")
		fmt.Println("[!] 错误信息:", err)
		<-ch
		fmt.Println("[+] 所有文件已下载完成.")
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second).C
		for range ticker {
			downloadingCount := 0
			waitingTask := -1
			for i, task := range downloaders {
				if task.Downloading {
					downloadingCount++
					bars[i].Set64(task.DownloadedSize)
				}
				if waitingTask == -1 && !task.Downloading && !task.Completed {
					waitingTask = i
				}
			}
			if downloadingCount < int(models.DefaultConf.PeakTaskCount) && waitingTask != -1 {
				task := downloaders[waitingTask]
				err := task.BeginDownload()
				if err != nil {
					//fmt.Println("[-] 文件", models.GetFileName(task.Info.TargetFile), "下载失败:", err)
					continue
				}
				task.OnCompleted(func() {
					bars[waitingTask].Finish()
				})
				task.OnFailed(func(err error) {
					bars[waitingTask].Finish()
				})
			}
			if downloadingCount == 0 && waitingTask == -1 {
				ch <- true
				break
			}
		}
	}()
	<-ch
	pool.Stop()
	time.Sleep(time.Second)
	fmt.Println("[+] 所有文件已下载完成.")
}

func (CommandHandler) DownloadCompleter(c *pl.Context) []string {
	if len(c.Nokeys) > 1 {
		return []string{}
	}
	return models.SelectStrings(append(filterCurrentDirs(), filterCurrentFiles()...), func(s string) string {
		if strings.Contains(s, " ") {
			return "\"" + s + "\""
		}
		return s
	})
}
