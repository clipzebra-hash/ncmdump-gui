package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SaveToType = string

const (
	Original SaveToType = "original"
	Custom   SaveToType = "custom"
)

type Preference struct {
	DeleteSource bool       `json:"delete_source"`
	SaveTo       SaveToType `json:"save_to"`
	Path         string     `json:"path"`
}

type ConfigManager struct {
	FilePath string
}

// NewConfigManager 创建一个新的配置管理器，自动适配不同操作系统的用户目录
func NewConfigManager(filename string) (*ConfigManager, error) {
	configDir, err := os.UserConfigDir() // 获取用户配置目录
	if err != nil {
		return nil, err
	}

	dirPath := filepath.Join(configDir, "ncmdump-gui")
	err = os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(dirPath, filename)

	configManager := &ConfigManager{FilePath: filePath}
	// if not exist, create it with default value
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		defaultConfig := &Preference{
			SaveTo: Original,
			Path:   "",
		}
		configManager.Save(defaultConfig)
	}
	return &ConfigManager{FilePath: filePath}, nil
}

// Save 将配置保存到文件
func (cm *ConfigManager) Save(preference *Preference) bool {
	file, err := os.Create(cm.FilePath)
	if err != nil {
		return false
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // 格式化输出
	err = encoder.Encode(preference)
	return err == nil
}

// Load 从文件读取配置
func (cm *ConfigManager) Load() *Preference {
	file, err := os.Open(cm.FilePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var preference *Preference = nil
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&preference)
	if err != nil {
		return nil
	}
	return preference
}
