package main

import (
	"strconv"
	"strings"
)

// 原工具的 config.yaml 是缩进嵌套结构，例如：
//
//	download:
//	  dir: "D:/videos/wx_channels"
//	proxy:
//	  port: 2023
//
// 早先的实现按 "download.dir:" 这种扁平键去逐行匹配，永远匹配不到，
// 于是「读不到配置」并且「保存设置静默失效」。
//
// 这里实现一个只覆盖本项目所需子集（缩进映射 + 标量）的 YAML 编辑器：
// 按 "a.b.c" 路径精确读写，同时完整保留注释、空行与原有排版。

type yamlLine struct {
	raw    string // 原始行（不含换行符）
	indent int    // 前导空格数
	key    string // 该行是 "key: value" 时的键名
	value  string // 冒号之后的原始内容（已去首尾空白，未去注释）
	path   string // 完整点分路径，例如 "channels.download.cover"
	hasKV  bool
}

type yamlDoc struct {
	lines []yamlLine
}

// parseYAML 把配置文本解析成带路径的行集合。
func parseYAML(content string) *yamlDoc {
	rawLines := strings.Split(content, "\n")
	doc := &yamlDoc{lines: make([]yamlLine, len(rawLines))}

	type frame struct {
		indent int
		key    string
	}
	var stack []frame

	for i, raw := range rawLines {
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		key, value, ok := splitKV(raw[indent:])
		line := yamlLine{raw: raw, indent: indent, hasKV: ok}
		if ok {
			// 同级或更浅的键出现时，弹出已结束的层级
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			parts := make([]string, 0, len(stack)+1)
			for _, f := range stack {
				parts = append(parts, f.key)
			}
			parts = append(parts, key)
			line.key = key
			line.value = value
			line.path = strings.Join(parts, ".")
			stack = append(stack, frame{indent: indent, key: key})
		}
		doc.lines[i] = line
	}
	return doc
}

// splitKV 只接受 "key: ..." 形式的行；列表项（"- xxx: yyy"）与注释行会被拒绝。
func splitKV(s string) (key, value string, ok bool) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = s[:idx]
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return "", "", false
		}
	}
	return key, strings.TrimSpace(s[idx+1:]), true
}

// stripYAMLComment 去掉行尾注释，但保留引号内的 #。
func stripYAMLComment(v string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '#' && !inSingle && !inDouble && (i == 0 || v[i-1] == ' ' || v[i-1] == '\t'):
			return strings.TrimSpace(v[:i])
		}
	}
	return strings.TrimSpace(v)
}

// unquoteYAML 去掉标量两端的引号，并还原双引号字符串里的转义。
func unquoteYAML(v string) string {
	v = strings.TrimSpace(v)
	if len(v) < 2 {
		return v
	}
	if v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	if v[0] == '"' && v[len(v)-1] == '"' {
		body := v[1 : len(v)-1]
		var b strings.Builder
		b.Grow(len(body))
		for i := 0; i < len(body); i++ {
			if body[i] == '\\' && i+1 < len(body) {
				switch body[i+1] {
				case '\\', '"':
					b.WriteByte(body[i+1])
					i++
					continue
				case 'n':
					b.WriteByte('\n')
					i++
					continue
				case 't':
					b.WriteByte('\t')
					i++
					continue
				}
			}
			b.WriteByte(body[i])
		}
		return b.String()
	}
	return v
}

// quoteYAML 输出双引号字符串。必须转义反斜杠，
// 否则 Windows 路径（D:\videos\...）会被 YAML 当成转义序列解析坏掉。
func quoteYAML(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return `"` + v + `"`
}

// get 返回指定路径的原始值。
func (d *yamlDoc) get(path string) (string, bool) {
	if idx, _ := d.findIndex(path); idx >= 0 {
		return d.lines[idx].value, true
	}
	return "", false
}

// getString 区分「键不存在」与「键存在但为空」，便于保留调用方的默认值。
func (d *yamlDoc) getString(path, def string) string {
	raw, ok := d.get(path)
	if !ok {
		return def
	}
	return unquoteYAML(stripYAMLComment(raw))
}

func (d *yamlDoc) getInt(path string, def int) int {
	raw, ok := d.get(path)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(unquoteYAML(stripYAMLComment(raw))))
	if err != nil {
		return def
	}
	return n
}

func (d *yamlDoc) getBool(path string, def bool) bool {
	raw, ok := d.get(path)
	if !ok {
		return def
	}
	v := strings.ToLower(strings.TrimSpace(unquoteYAML(stripYAMLComment(raw))))
	switch v {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0":
		return false
	}
	return def
}

// set 按路径写值：已存在则就地替换该行，否则在父节点块内追加（父节点不存在时递归创建）。
func (d *yamlDoc) set(path, rawValue string) {
	suffix := ""
	if rawValue != "" {
		suffix = " " + rawValue
	}

	if idx, _ := d.findIndex(path); idx >= 0 {
		d.lines[idx].raw = strings.Repeat(" ", d.lines[idx].indent) + d.lines[idx].key + ":" + suffix
		d.lines[idx].value = rawValue
		return
	}

	parentPath, leaf := splitPath(path)
	parentIdx, parentIndent := -1, -1

	if parentPath != "" {
		parentIdx, parentIndent = d.findIndex(parentPath)
		if parentIdx == -1 {
			d.set(parentPath, "")
			// 新建的父节点需要重新计算路径，才能继续往里插子键
			*d = *parseYAML(d.String())
			parentIdx, parentIndent = d.findIndex(parentPath)
			if parentIdx == -1 {
				return
			}
		}
	}
	insertIndent := 0
	insertAt := len(d.lines)
	if parentIdx >= 0 {
		insertIndent = parentIndent + 2
		insertAt = parentIdx + 1
		// 落在父节点块的最后一行之后
		for j := parentIdx + 1; j < len(d.lines); j++ {
			if strings.TrimSpace(d.lines[j].raw) == "" {
				continue
			}
			if d.lines[j].indent > parentIndent {
				insertAt = j + 1
				continue
			}
			break
		}
	}

	newLine := yamlLine{
		raw:    strings.Repeat(" ", insertIndent) + leaf + ":" + suffix,
		indent: insertIndent,
		key:    leaf,
		value:  rawValue,
		path:   path,
		hasKV:  true,
	}
	d.lines = append(d.lines, yamlLine{})
	copy(d.lines[insertAt+1:], d.lines[insertAt:])
	d.lines[insertAt] = newLine
}

func (d *yamlDoc) findIndex(path string) (idx, indent int) {
	for i := range d.lines {
		if d.lines[i].hasKV && d.lines[i].path == path {
			return i, d.lines[i].indent
		}
	}
	return -1, -1
}

func splitPath(path string) (parent, leaf string) {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[:idx], path[idx+1:]
	}
	return "", path
}

func (d *yamlDoc) String() string {
	parts := make([]string, len(d.lines))
	for i := range d.lines {
		parts[i] = d.lines[i].raw
	}
	return strings.Join(parts, "\n")
}

// yamlInt / yamlStr / yamlBool 等旧接口已由 yamlDoc 取代，
// 为避免误用，这里不再保留同名函数。
