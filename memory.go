package deepbot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cohesion-org/deepseek-go"
	"github.com/wdvxdr1123/ZeroBot"
)

// ===================== 消息类型（用于拉取群聊历史） =====================

type msgType struct {
	UserID  uint64 `json:"user_id"`
	GroupID uint64 `json:"group_id"`
	SubType string `json:"sub_type"`
	Time    uint64 `json:"time"`
	Sender  struct {
		UserID   uint64 `json:"user_id"`
		Nickname string `json:"nickname"`
		Card     string `json:"card,omitempty"`
		Role     string `json:"role,omitempty"`
		Sex      string `json:"sex,omitempty"`
		Age      uint64 `json:"age,omitempty"`
	} `json:"sender"`
	Message []struct {
		Type string `json:"type"`
		Data struct {
			Text    string `json:"text,omitempty"`
			Name    string `json:"name,omitempty"`
			QQ      string `json:"qq"`
			ID      string `json:"id"`
			File    string `json:"file,omitempty"`
			Summary string `json:"summary,omitempty"`
			Data    string `json:"data,omitempty"`
			Content any    `json:"content,omitempty"`
		} `json:"data"`
	} `json:"message"`
	MessageID       uint64 `json:"message_id"`
	MessageSeq      uint64 `json:"message_seq"`
	MessageFormat   string `json:"message_format"`
	MessageType     string `json:"message_type"`
	MessageSentType string `json:"message_sent_type"`
	PostType        string `json:"post_type"`
	RealID          uint64 `json:"real_id"`
	SelfID          uint64 `json:"self_id"`
	RawMessage      string `json:"raw_message"`
	Font            uint64 `json:"font"`
}

type msgItem struct {
	MessageID uint64 `json:"message_id"`
	DateTime  string `json:"date_time"`
	UserName  string `json:"user_name"`
	UserID    uint64 `json:"user_id"`
	Content   string `json:"content"`
}

// ===================== 记忆数据结构 =====================

type memoryEntry struct {
	UserID   uint64 `json:"user_id"`
	UserName string `json:"user_name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	DateTime string `json:"date_time"`
}

type memoryRef struct {
	File     string `json:"file"`
	UserID   uint64 `json:"user_id"`
	UserName string `json:"user_name"`
	Type     string `json:"type"`
	Summary  string `json:"summary"`
}

type memoryIndex struct {
	GroupID int64       `json:"group_id"`
	Updated string      `json:"updated"`
	Entries []memoryRef `json:"entries"`
}

const (
	memMaxIndexLines = 200
	memMaxIndexBytes = 25 * 1024
	memMaxInject     = 5
)

var memLocks sync.Map // map[int64]*sync.Mutex

func memLock(groupID int64) *sync.Mutex {
	v, _ := memLocks.LoadOrStore(groupID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ===================== 目录与索引管理 =====================

func ensureMemDir(groupID int64) string {
	dir := fmt.Sprintf("data/memory/%d", groupID)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (bot *DeepBot) loadMemoryIndex(groupID int64) *memoryIndex {
	path := fmt.Sprintf("data/memory/%d/index.md", groupID)
	data, err := os.ReadFile(path)
	if err != nil {
		return &memoryIndex{GroupID: groupID}
	}
	idx := &memoryIndex{}
	err = jsonDecode(data, idx)
	if err != nil {
		log.Println("failed to decode memory index:", err)
		return &memoryIndex{GroupID: groupID}
	}
	return idx
}

func (bot *DeepBot) saveMemoryIndex(idx *memoryIndex) {
	groupID := idx.GroupID
	ensureMemDir(groupID)

	data, err := jsonEncode(idx)
	if err != nil {
		log.Println("failed to encode memory index:", err)
		return
	}
	// 截断控制
	lines := strings.Split(string(data), "\n")
	if len(lines) > memMaxIndexLines || len(data) > memMaxIndexBytes {
		log.Printf("[warning] memory index for group %d exceeds limit (lines=%d, bytes=%d)\n",
			groupID, len(lines), len(data))
	}
	path := fmt.Sprintf("data/memory/%d/index.md", groupID)
	err = os.WriteFile(path, data, 0600)
	if err != nil {
		log.Println("failed to save memory index:", err)
	}
}

// ===================== 记忆文件读写 =====================

func (bot *DeepBot) saveMemoryEntries(groupID int64, entries []memoryEntry) []memoryRef {
	mu := memLock(groupID)
	mu.Lock()
	defer mu.Unlock()

	idx := bot.loadMemoryIndex(groupID)
	dir := ensureMemDir(groupID)
	existing := make(map[string]bool)
	for _, ref := range idx.Entries {
		existing[ref.Summary] = true
	}

	nextID := len(idx.Entries) + 1
	var refs []memoryRef
	for _, entry := range entries {
		// 简单去重：摘要相同就跳过
		summary := truncateText(entry.Content, 80)
		if existing[summary] {
			continue
		}
		existing[summary] = true

		file := fmt.Sprintf("%03d.md", nextID)
		nextID++

		content := fmt.Sprintf("---\nuser_id: %d\nuser_name: %s\ntype: %s\ndate: %s\n---\n%s\n",
			entry.UserID, entry.UserName, entry.Type, entry.DateTime, entry.Content)
		path := filepath.Join(dir, file)
		err := os.WriteFile(path, []byte(content), 0600)
		if err != nil {
			log.Println("failed to save memory entry:", err)
			continue
		}

		ref := memoryRef{
			File:     file,
			UserID:   entry.UserID,
			UserName: entry.UserName,
			Type:     entry.Type,
			Summary:  summary,
		}
		refs = append(refs, ref)
		idx.Entries = append(idx.Entries, ref)
	}

	idx.Updated = time.Now().Format(time.DateTime)
	bot.saveMemoryIndex(idx)
	return refs
}

func (bot *DeepBot) loadMemoryEntry(groupID int64, file string) (*memoryEntry, error) {
	path := fmt.Sprintf("data/memory/%d/%s", groupID, file)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseMemoryFile(content)
}

func parseMemoryFile(data []byte) (*memoryEntry, error) {
	text := string(data)
	// 解析 YAML frontmatter
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("invalid memory file: no frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("invalid memory file: unclosed frontmatter")
	}
	fm := text[4 : 4+end]
	body := text[4+end+5:]

	entry := &memoryEntry{Content: strings.TrimSpace(body)}
	for _, line := range strings.Split(fm, "\n") {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "user_id":
			fmt.Sscanf(val, "%d", &entry.UserID)
		case "user_name":
			entry.UserName = val
		case "type":
			entry.Type = val
		case "date":
			entry.DateTime = val
		}
	}
	return entry, nil
}

// ===================== 记忆检索 =====================

func (bot *DeepBot) getMemoriesForPrompt(groupID int64) string {
	if !bot.config.Memory.Enabled || groupID == 0 {
		return ""
	}
	idx := bot.loadMemoryIndex(groupID)
	if len(idx.Entries) == 0 {
		return ""
	}
	var lines []string
	maxN := bot.config.Memory.MaxInject
	if maxN <= 0 {
		maxN = memMaxInject
	}
	if len(idx.Entries) > maxN {
		// 取最新的 N 条
		entries := idx.Entries[len(idx.Entries)-maxN:]
		for _, ref := range entries {
			lines = append(lines, fmt.Sprintf("- %s: %s", ref.UserName, ref.Summary))
		}
	} else {
		for _, ref := range idx.Entries {
			lines = append(lines, fmt.Sprintf("- %s: %s", ref.UserName, ref.Summary))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "[你已知的群聊记忆]\n" + strings.Join(lines, "\n")
}

// ===================== 群聊总结 → 记忆提取 =====================

const promptSummarize = `
[角色] 你是一个记忆提取助手。

[任务] 从以下群聊记录中提取有价值的记忆。每条记忆包含：日期时间、user_id、user_name、类型、内容。

[记忆类型]
- personal_info: 个人信息（姓名、年龄、职业、所在地、宠物等）
- habit: 生活习惯（作息、饮食、运动等）
- interest: 兴趣爱好（喜欢什么、在学什么等）
- event: 近期事件（发生了什么、计划做什么等）
- preference: 偏好（喜欢/讨厌什么、观点态度等）
- emotion: 表达的情绪状态

[输出格式] 每条记忆一行，用 "|" 分隔字段：
日期时间 | user_id | user_name | 类型 | 记忆内容

[注意事项]
- 只提取确实有信息价值的记忆，忽略日常闲聊、打招呼、表情包等无信息内容
- 如果聊天记录中没有值得记录的实质性信息，输出"无新记忆"
- 不要编造不存在的信息

========================群聊记录========================
`

func (bot *DeepBot) onSummarizeGroupMsg(ctx *zero.Ctx) {
	bot.summarizeAndStore(ctx)
}

func (bot *DeepBot) summarizeAndStore(ctx *zero.Ctx) {
	groupID := ctx.Event.GroupID
	if groupID == 0 {
		bot.sendText(ctx, "此命令仅支持群聊")
		return
	}

	items := bot.fetchGroupMessages(ctx)
	if len(items) == 0 {
		bot.sendText(ctx, "获取群聊记录失败")
		return
	}

	output, err := jsonEncode(items)
	if err != nil {
		log.Println("failed to encode history message:", err)
		return
	}

	prompt := promptSummarize + string(output)
	fmt.Println("memory summarize prompt length:", len(prompt))

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 0,
		TopP:        1,
		MaxTokens:   8192,
	}
	user := new(user)
	resp, err := bot.seek(req, user, prompt)
	if err != nil {
		log.Println("failed to summarize memory:", err)
		bot.sendText(ctx, "记忆总结失败")
		return
	}

	answer := strings.TrimSpace(resp.Answer)
	fmt.Println("memory summarize result:", answer)

	if answer == "无新记忆" || answer == "" {
		bot.sendText(ctx, "未发现新的有价值的记忆")
		return
	}

	entries := parseMemoryAnswer(answer)
	if len(entries) == 0 {
		bot.sendText(ctx, "未发现新的有价值的记忆")
		return
	}

	refs := bot.saveMemoryEntries(groupID, entries)
	bot.sendText(ctx, fmt.Sprintf("已总结出 %d 条新记忆", len(refs)))
}

func (bot *DeepBot) fetchGroupMessages(ctx *zero.Ctx) []*msgItem {
	params := make(zero.Params)
	params["group_id"] = ctx.Event.GroupID
	params["message_seq"] = 0
	params["count"] = 300

	resp := ctx.CallAction("get_group_msg_history", params)
	if resp.Status != "ok" {
		return nil
	}

	var messages []*msgType
	raw := resp.Data.Get("messages").Raw
	err := jsonDecode([]byte(raw), &messages)
	if err != nil {
		log.Println("failed to read group history message:", err)
		return nil
	}

	var items []*msgItem
	for _, msg := range messages {
		var content string
		for _, m := range msg.Message {
			switch m.Type {
			case "text":
				content += fmt.Sprintf("[text]: %s\n", m.Data.Text)
			case "at":
				content += fmt.Sprintf("[at]: %s\n", m.Data.QQ)
			case "reply":
				content += fmt.Sprintf("[reply]: %s\n", m.Data.ID)
			}
		}
		if content == "" {
			continue
		}
		dateTime := time.Unix(int64(msg.Time), 0).Local().Format(time.DateTime)
		userName := msg.Sender.Nickname
		if msg.Sender.Card != "" {
			userName = msg.Sender.Card
		}
		item := &msgItem{
			MessageID: msg.MessageID,
			DateTime:  dateTime,
			UserName:  userName,
			UserID:    msg.Sender.UserID,
			Content:   content,
		}
		items = append(items, item)
	}
	return items
}

func parseMemoryAnswer(answer string) []memoryEntry {
	var entries []memoryEntry
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		var uid uint64
		fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &uid)
		entry := memoryEntry{
			DateTime: strings.TrimSpace(parts[0]),
			UserID:   uid,
			UserName: strings.TrimSpace(parts[2]),
			Type:     strings.TrimSpace(parts[3]),
			Content:  strings.TrimSpace(parts[4]),
		}
		entries = append(entries, entry)
	}
	return entries
}

// ===================== 记忆整理（Auto Dream） =====================

const promptConsolidate = `
[角色] 你是一个记忆整理助手。

[任务] 审查以下群聊记忆，找出需要处理的条目：
1. 重复的记忆 → 标记为 "dup: <条目序号>" (保留最早的一条)
2. 明显过时/已被新事实覆盖的记忆 → 标记为 "stale: <条目序号>"
3. 内容相似的记忆 → 标记为 "merge: <序号A>, <序号B>" (建议保留A)

[输出格式] 每行一条操作，无操作则输出 "noop"

========================记忆列表========================
`

func (bot *DeepBot) consolidateGroupMemory(groupID int64) {
	mu := memLock(groupID)
	mu.Lock()
	defer mu.Unlock()

	idx := bot.loadMemoryIndex(groupID)
	if len(idx.Entries) == 0 {
		return
	}

	// Phase 1-2: 定位 + 收集
	var lines []string
	for i, ref := range idx.Entries {
		entry, err := bot.loadMemoryEntry(groupID, ref.File)
		if err != nil {
			log.Printf("failed to load memory entry %s: %s\n", ref.File, err)
			continue
		}
		lines = append(lines, fmt.Sprintf("%d | %s | %s | %s | %s",
			i+1, entry.DateTime, entry.UserName, entry.Type, entry.Content))
	}
	if len(lines) == 0 {
		return
	}

	prompt := promptConsolidate + strings.Join(lines, "\n")

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 0,
		TopP:        1,
		MaxTokens:   4096,
	}
	emptyUser := new(user)
	resp, err := bot.seek(req, emptyUser, prompt)
	if err != nil {
		log.Println("failed to consolidate memory:", err)
		return
	}

	// Phase 3-4: 整合 + 修剪
	answer := strings.TrimSpace(resp.Answer)
	if answer == "noop" || answer == "" {
		idx.Updated = time.Now().Format(time.DateTime)
		bot.saveMemoryIndex(idx)
		return
	}

	removeSet := make(map[int]bool)
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "dup:"):
			var n int
			fmt.Sscanf(strings.TrimPrefix(line, "dup:"), "%d", &n)
			if n > 1 && n <= len(idx.Entries) {
				removeSet[n-1] = true
			}
		case strings.HasPrefix(line, "stale:"):
			var n int
			fmt.Sscanf(strings.TrimPrefix(line, "stale:"), "%d", &n)
			if n > 0 && n <= len(idx.Entries) {
				removeSet[n-1] = true
			}
		}
	}

	if len(removeSet) > 0 {
		var kept []memoryRef
		for i, ref := range idx.Entries {
			if removeSet[i] {
				path := fmt.Sprintf("data/memory/%d/%s", groupID, ref.File)
				_ = os.Remove(path)
			} else {
				kept = append(kept, ref)
			}
		}
		idx.Entries = kept

		// 重新编号文件
		for i, ref := range idx.Entries {
			newFile := fmt.Sprintf("%03d.md", i+1)
			if ref.File != newFile {
				oldPath := fmt.Sprintf("data/memory/%d/%s", groupID, ref.File)
				newPath := fmt.Sprintf("data/memory/%d/%s", groupID, newFile)
				_ = os.Rename(oldPath, newPath)
				ref.File = newFile
			}
		}
	}

	idx.Updated = time.Now().Format(time.DateTime)
	bot.saveMemoryIndex(idx)
}

// ===================== 定时器 =====================

func (bot *DeepBot) autoMemoryLoop() {
	interval := bot.config.Memory.AutoInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		for _, groupID := range bot.config.GroupID {
			bot.consolidateGroupMemory(groupID)
		}
	}
}

// ===================== 辅助函数 =====================

func truncateText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

