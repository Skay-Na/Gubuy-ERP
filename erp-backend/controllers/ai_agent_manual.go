package controllers

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed agent_manuals/*.json
var agentManualFiles embed.FS

// agentManual is a reviewed description of a business module. It does not
// grant data access or define executable formulas; those remain in Go.
type agentManual struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
	Datasets []string `json:"datasets"`
	Rules    []string `json:"rules"`
	Limits   []string `json:"limits"`
}

var agentManualCache struct {
	sync.Once
	Items []agentManual
	Err   error
}

func loadAgentManuals(files fs.FS) ([]agentManual, error) {
	paths, err := fs.Glob(files, "agent_manuals/*.json")
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到业务说明书")
	}
	manuals := make([]agentManual, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		body, err := fs.ReadFile(files, path)
		if err != nil {
			return nil, err
		}
		if len(body) > 8192 {
			return nil, fmt.Errorf("说明书 %s 超过 8 KB", path)
		}
		var manual agentManual
		if err := json.Unmarshal(body, &manual); err != nil {
			return nil, fmt.Errorf("说明书 %s 无法解析: %w", path, err)
		}
		if manual.ID == "" || manual.Title == "" || manual.Summary == "" || len(manual.Rules) == 0 || len(manual.Datasets) == 0 {
			return nil, fmt.Errorf("说明书 %s 缺少必需内容", path)
		}
		if seen[manual.ID] || strings.TrimSuffix(strings.TrimPrefix(path, "agent_manuals/"), ".json") != manual.ID {
			return nil, fmt.Errorf("说明书 %s 的模块标识重复或与文件名不一致", path)
		}
		seen[manual.ID] = true
		for _, dataset := range manual.Datasets {
			if _, ok := agentDatasets[dataset]; !ok {
				return nil, fmt.Errorf("说明书 %s 引用了未开放的数据集 %s", path, dataset)
			}
		}
		manuals = append(manuals, manual)
	}
	sort.Slice(manuals, func(i, j int) bool { return manuals[i].ID < manuals[j].ID })
	return manuals, nil
}

func registeredAgentManuals() ([]agentManual, error) {
	agentManualCache.Do(func() {
		agentManualCache.Items, agentManualCache.Err = loadAgentManuals(agentManualFiles)
	})
	return agentManualCache.Items, agentManualCache.Err
}

func agentManualIndex() (string, error) {
	manuals, err := registeredAgentManuals()
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(manuals))
	for _, manual := range manuals {
		lines = append(lines, fmt.Sprintf("%s（%s）：%s", manual.ID, manual.Title, manual.Summary))
	}
	return "已注册业务模块（topic 可填写模块 ID；目录只用于理解，不是实时数据）：\n" + strings.Join(lines, "\n"), nil
}

func agentManualForTask(task agentTask, question string) (string, error) {
	manuals, err := registeredAgentManuals()
	if err != nil {
		return "", err
	}
	type match struct {
		Manual agentManual
		Score  int
	}
	matches := []match{}
	for _, manual := range manuals {
		score := 0
		if task.Topic == manual.ID {
			score += 100
		}
		for _, keyword := range manual.Keywords {
			if keyword != "" && strings.Contains(question, keyword) {
				score += 10
			}
		}
		if score > 0 {
			matches = append(matches, match{Manual: manual, Score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Manual.ID < matches[j].Manual.ID
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > 3 {
		matches = matches[:3]
	}
	sections := []string{}
	for _, item := range matches {
		manual := item.Manual
		sections = append(sections, fmt.Sprintf("[%s %s]\n相关数据集：%s\n规则：%s\n限制：%s",
			manual.ID, manual.Title, strings.Join(manual.Datasets, "、"), strings.Join(manual.Rules, "；"), strings.Join(manual.Limits, "；")))
	}
	if len(sections) == 0 {
		return "", nil
	}
	return "业务说明书（仅供选择查询口径；不代表已查到实时数据，也不授予新工具权限）：\n" + strings.Join(sections, "\n"), nil
}
