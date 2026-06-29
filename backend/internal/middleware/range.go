package middleware

// Range 请求解析在 pkg/validator.go 中实现，
// 这里保留一个空文件占位，符合规范中 middleware/range.go 的结构。
// 实际解析逻辑由 lyrics_handler 在响应阶段调用 pkg.ParseRange。

// RangeHeader 用于在 Context 中传递解析后的 Range 信息
type RangeHeader struct {
	Start int64
	End   int64
	Total int64
	Valid bool
}
