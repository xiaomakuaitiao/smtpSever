package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type Config struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
	} `yaml:"database"`
	Settings struct {
		SingleRecipientOnly bool   `yaml:"singleRecipientOnly"`
		MaxMessageBytes     int64  `yaml:"maxMessageBytes"`
		MaxRecipients       int    `yaml:"maxRecipients"`
		ReadTimeout         int    `yaml:"readTimeout"`
		WriteTimeout        int    `yaml:"writeTimeout"`
		CertPath            string `yaml:"certPath"`
		KeyPath             string `yaml:"keyPath"`
		Domain              string `yaml:"domain"`
		Addr                string `yaml:"addr"`
		TLSAddr             string `yaml:"tlsAddr"`
	} `yaml:"settings"`
}

// LoadConfig 从文件中加载配置
func LoadConfig(configFile string) (*Config, error) {
	config := &Config{}
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
