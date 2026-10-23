package main

import (
	"log/slog"

	"claude2api/internal/config"
	"claude2api/internal/models"
	"claude2api/internal/repository"
	"claude2api/internal/router"
	"claude2api/internal/service"
)

func main() {
	config.Load()
	// 恢复上次从 claude.ai 网页端拉取的模型缓存。
	models.LoadDynamic()
	if err := repository.InitDB(); err != nil {
		slog.Error("[启动] 数据库初始化失败", "err", err)
		return
	}
	service.StartAccountStatusMonitor()
	service.StartModelSyncLoop()
	router.RunServer()
}
