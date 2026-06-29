package pkg

import (
	"net/http"
	"strconv"
	"strings"
)

// ParseInt 解析整数参数，失败返回默认值
func ParseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// Clamp 将 n 限制在 [min, max] 区间
func Clamp(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// IsValidFolder 校验 folder 是否合法
func IsValidFolder(folder string) bool {
	switch folder {
	case "raw-lyrics", "ncm-lyrics", "qq-lyrics", "spotify-lyrics", "am-lyrics":
		return true
	}
	return false
}

// IsValidPlatform 校验 platform 是否合法
func IsValidPlatform(p string) bool {
	switch p {
	case "ncm", "qq", "spotify", "apple":
		return true
	}
	return false
}

// FolderToPlatform folder 名称转 platform 标识
// ncm-lyrics -> ncm, am-lyrics -> apple, etc.
func FolderToPlatform(folder string) string {
	switch folder {
	case "raw-lyrics":
		return ""
	case "ncm-lyrics":
		return "ncm"
	case "qq-lyrics":
		return "qq"
	case "spotify-lyrics":
		return "spotify"
	case "am-lyrics":
		return "apple"
	}
	return ""
}

// IsHTTPRequestIDEmpty 用于判断 request id 是否已生成
func IsHTTPRequestIDEmpty(s string) bool { return strings.TrimSpace(s) == "" }

// HTTPRange 表示一个 HTTP Range 请求
type HTTPRange struct {
	Start  int64
	End    int64
	Total  int64
	Valid  bool
}

// ParseRange 解析 HTTP Range 请求头，格式：bytes=start-end
// 支持：bytes=0-1024 / bytes=0- / bytes=-1024
func ParseRange(header string, total int64) HTTPRange {
	r := HTTPRange{Total: total}
	if header == "" {
		return r
	}
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return r
	}
	spec := strings.TrimPrefix(header, prefix)
	// 仅支持单一范围
	if strings.Contains(spec, ",") {
		spec = strings.TrimSpace(strings.Split(spec, ",")[0])
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return r
	}
	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	if startStr == "" {
		// suffix range: bytes=-N
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return r
		}
		if n > total {
			n = total
		}
		r.Start = total - n
		r.End = total - 1
	} else {
		start, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			return r
		}
		if start >= total {
			return r
		}
		r.Start = start
		if endStr == "" {
			r.End = total - 1
		} else {
			end, err := strconv.ParseInt(endStr, 10, 64)
			if err != nil {
				return r
			}
			if end >= total {
				end = total - 1
			}
			if end < start {
				return r
			}
			r.End = end
		}
	}
	r.Valid = true
	return r
}

// ContentRangeHeader 生成 Content-Range 头，如 "bytes 0-1024/2847"
func (r HTTPRange) ContentRangeHeader() string {
	return "bytes " + strconv.FormatInt(r.Start, 10) + "-" + strconv.FormatInt(r.End, 10) + "/" + strconv.FormatInt(r.Total, 10)
}

// Length 返回当前 Range 的字节数
func (r HTTPRange) Length() int64 {
	return r.End - r.Start + 1
}

// StatusOutOfRange 返回 416 状态码
func HTTPRangeOutOfRange() int { return http.StatusRequestedRangeNotSatisfiable }
