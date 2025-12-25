package http

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func GetString(q url.Values, key string, def string) string {
	if v := q.Get(key); v != "" {
		return v
	}
	return def
}

func GetInt(q url.Values, key string, def int) int {
	if v := q.Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func GetInt64(q url.Values, key string) *int64 {
	if v := q.Get(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return &n
		}
	}
	return nil
}

func GetUUID(q url.Values, key string) *uuid.UUID {
	if v := q.Get(key); v != "" {
		if u, err := uuid.Parse(v); err == nil {
			return &u
		}
	}
	return nil
}

func GetBool(q url.Values, key string) bool {
	return q.Get(key) == "true"
}

func GetBoolPtr(q url.Values, key string) *bool {
	v := q.Get(key)
	if v == "" {
		return nil
	}

	if v == "true" {
		b := true
		return &b
	}

	if v == "false" {
		b := false
		return &b
	}

	return nil
}

func GetStringSlice(q url.Values, key string) []string {
	arr := []string{}
	for s := range strings.SplitSeq(q.Get(key), ",") {
		if s = strings.TrimSpace(s); s != "" {
			arr = append(arr, s)
		}
	}
	return arr
}
