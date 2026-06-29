package handler

import "time"

// defaultTimeout 默认请求超时
const defaultTimeout = 10 * time.Second

// longTimeout 用于流式/外部请求
const longTimeout = 60 * time.Second
