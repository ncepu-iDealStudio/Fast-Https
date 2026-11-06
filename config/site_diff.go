package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"fast-https/infra/jsonc"
)

// PatchSiteFile 把一个站点文件从 old 改成 want，返回新的文件内容。
//
// 关键点：不是把 want 整份序列化回去，而是先算出两者之间"真正变了"的字段，
// 再用 infra/jsonc 按路径就地替换。于是文件里那些 site 这一层根本没有建模的
// 字段——auth、proxy_cache、appfirewall、limit、zip、try、error_page——以及
// 注释、缩进、字段顺序都原样留着。
func PatchSiteFile(raw []byte, old, want SiteInput) ([]byte, error) {
	if len(want.Locations) < len(old.Locations) {
		return nil, fmt.Errorf("不支持通过 site 命令删除 location（现有 %d 个，目标 %d 个）",
			len(old.Locations), len(want.Locations))
	}

	edits := siteDiffEdits(old, want)
	if len(edits) == 0 {
		return raw, nil
	}

	// location 在文件里可能根本不存在，追加元素前先补一个空数组。
	if needsLocationArray(raw, edits) {
		edits = append([]jsonc.Edit{{
			Path:  []jsonc.Step{jsonc.Key("location")},
			Value: []byte("[]"),
		}}, edits...)
	}

	return jsonc.PatchPath(raw, edits)
}

// siteDiffEdits 逐字段比对，只产出确实变化的路径。
func siteDiffEdits(old, want SiteInput) []jsonc.Edit {
	var edits []jsonc.Edit

	if !sameStrings(old.Listens, want.Listens) {
		edits = append(edits, setEdit("listen", listenEditValue(want.Listens)))
	}
	if !sameText(old.ServerName, want.ServerName) {
		edits = append(edits, setEdit("server_name", mustJSON(want.ServerName)))
	}
	edits = append(edits, stringFieldEdits("ssl_certificate", old.SSLCertificate, want.SSLCertificate)...)
	edits = append(edits, stringFieldEdits("ssl_certificate_key", old.SSLCertificateKey, want.SSLCertificateKey)...)

	for i := range old.Locations {
		if i >= len(want.Locations) {
			break
		}
		before, after := old.Locations[i], want.Locations[i]
		base := []jsonc.Step{jsonc.Key("location"), jsonc.At(i)}

		if !sameText(before.URL, after.URL) && strings.TrimSpace(after.URL) != "" {
			edits = append(edits, setEditAt(base, "url", mustJSON(after.URL)))
		}
		if !sameText(before.Type, after.Type) && strings.TrimSpace(after.Type) != "" {
			edits = append(edits, setEditAt(base, "type", mustJSON(after.Type)))
		}
		edits = append(edits, stringFieldEditsAt(base, "root", before.Root, after.Root)...)
		edits = append(edits, stringFieldEditsAt(base, "proxy_pass", before.ProxyPass, after.ProxyPass)...)
		if !sameStrings(before.Index, after.Index) {
			if len(after.Index) == 0 {
				edits = append(edits, delEditAt(base, "index"))
			} else {
				edits = append(edits, setEditAt(base, "index", mustJSON(after.Index)))
			}
		}
	}

	// 新增的 location 追加到数组末尾。用两空格缩进生成，插入后与文件其余部分对齐。
	for i := len(old.Locations); i < len(want.Locations); i++ {
		value, err := json.MarshalIndent(want.Locations[i], "", "  ")
		if err != nil {
			continue
		}
		edits = append(edits, jsonc.Edit{
			Path:  []jsonc.Step{jsonc.Key("location"), jsonc.Append()},
			Value: value,
		})
	}
	return edits
}

// stringFieldEdits 处理值可能被清空的字段：新值为空且旧值非空时删掉这个键，
// 而不是留一个空字符串——空串会参与校验，删掉才是"回到未配置"。
func stringFieldEdits(key, before, after string) []jsonc.Edit {
	return stringFieldEditsAt(nil, key, before, after)
}

func stringFieldEditsAt(base []jsonc.Step, key, before, after string) []jsonc.Edit {
	if sameText(before, after) {
		return nil
	}
	if strings.TrimSpace(after) == "" {
		return []jsonc.Edit{delEditAt(base, key)}
	}
	return []jsonc.Edit{setEditAt(base, key, mustJSON(after))}
}

func setEdit(key string, value []byte) jsonc.Edit {
	return jsonc.Edit{Path: []jsonc.Step{jsonc.Key(key)}, Value: value}
}

func setEditAt(base []jsonc.Step, key string, value []byte) jsonc.Edit {
	return jsonc.Edit{Path: pathWith(base, key), Value: value}
}

func delEditAt(base []jsonc.Step, key string) jsonc.Edit {
	return jsonc.Edit{Path: pathWith(base, key)}
}

func pathWith(base []jsonc.Step, key string) []jsonc.Step {
	out := make([]jsonc.Step, 0, len(base)+1)
	out = append(out, base...)
	return append(out, jsonc.Key(key))
}

// needsLocationArray 判断这次修改是否需要先补出 location 键。
func needsLocationArray(raw []byte, edits []jsonc.Edit) bool {
	touchesLocation := false
	for _, e := range edits {
		if len(e.Path) > 0 && !e.Path[0].IsIndex && e.Path[0].Key == "location" {
			touchesLocation = true
			break
		}
	}
	if !touchesLocation {
		return false
	}

	masked := jsonc.StripComments(raw)
	obj, err := jsonc.ScanObject(masked)
	if err != nil {
		return false
	}
	for _, m := range obj.Members {
		if m.Key == "location" {
			return false
		}
	}
	return true
}

// listenEditValue 与 MarshalJSONFile 用同一套编码：单条写成标量，多条写成数组。
func listenEditValue(listens []string) []byte {
	normalized := normalizeListens(listens)
	if len(normalized) == 0 {
		normalized = []string{"80"}
	}
	return mustJSON(flexList(normalized))
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return data
}

func sameText(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
