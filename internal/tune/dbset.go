package tune

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The settings of the database server a person may change from the panel. They go into a file of their own that sorts after mpn's
// tuning, so a value here wins and a reinstall that rewrites 99-mpn.cnf keeps them. Only these keys, with numbers
// in a range or one of a few words, can be set: a free text box would let one typo stop the server.

// DBSetting is one setting the panel offers.
type DBSetting struct {
	Key     string   `json:"key"`
	Unit    string   `json:"unit,omitempty"` // "M" for megabytes, "s" for seconds, "" for a plain number or a word
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// DBSettings are the settings in the order the page shows them.
var DBSettings = []DBSetting{
	{Key: "max_connections", Min: 10, Max: 5000},
	{Key: "innodb_buffer_pool_size", Unit: "M", Min: 32, Max: 262144},
	{Key: "max_allowed_packet", Unit: "M", Min: 1, Max: 1024},
	{Key: "wait_timeout", Unit: "s", Min: 10, Max: 31536000},
	{Key: "interactive_timeout", Unit: "s", Min: 10, Max: 31536000},
	{Key: "tmp_table_size", Unit: "M", Min: 1, Max: 4096},
	{Key: "max_heap_table_size", Unit: "M", Min: 1, Max: 4096},
	{Key: "table_open_cache", Min: 64, Max: 100000},
	{Key: "slow_query_log", Choices: []string{"ON", "OFF"}},
	{Key: "long_query_time", Unit: "s", Min: 0, Max: 3600},
	{Key: "sql_mode", Choices: []string{"default", "strict", "relaxed"}},
}

// sqlModes are what the three words of sql_mode mean. "default" leaves the server's own value (the line is not written).
var sqlModes = map[string]string{
	"strict":  "STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION",
	"relaxed": "NO_ENGINE_SUBSTITUTION", // for old applications that insert empty strings into number columns
}

func settingOf(key string) (DBSetting, bool) {
	for _, s := range DBSettings {
		if s.Key == key {
			return s, true
		}
	}
	return DBSetting{}, false
}

// CheckDBValues returns the values as they will be written, or the first problem. An empty value means "leave it to the server" and
// is dropped. A key that is not in DBSettings is refused.
func CheckDBValues(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range in {
		v = strings.TrimSpace(v)
		s, ok := settingOf(k)
		if !ok {
			return nil, fmt.Errorf("%s cannot be set from the panel", k)
		}
		if v == "" || (k == "sql_mode" && v == "default") {
			continue
		}
		if len(s.Choices) > 0 {
			found := false
			for _, c := range s.Choices {
				if strings.EqualFold(v, c) {
					v, found = c, true
				}
			}
			if !found {
				return nil, fmt.Errorf("%s must be one of %s", k, strings.Join(s.Choices, ", "))
			}
			out[k] = v
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(v, "M"), "s"))
		if err != nil || n < s.Min || n > s.Max {
			return nil, fmt.Errorf("%s must be a whole number from %d to %d", k, s.Min, s.Max)
		}
		out[k] = strconv.Itoa(n)
	}
	return out, nil
}

// UserDBConf writes checked values as the [mysqld] file. engine matters for one key only: slow_query_log needs a file on MariaDB and
// MySQL alike, which is put in the server's own log folder.
func UserDBConf(values map[string]string) string {
	var b strings.Builder
	b.WriteString("# Made by mpn from the panel (Services > Database settings). Changes made here are overwritten.\n[mysqld]\n")
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool { return indexOf(keys[i]) < indexOf(keys[j]) })
	for _, k := range keys {
		s, _ := settingOf(k)
		v := values[k]
		switch {
		case k == "sql_mode":
			fmt.Fprintf(&b, "sql_mode = \"%s\"\n", sqlModes[v])
		case s.Unit == "M":
			fmt.Fprintf(&b, "%s = %sM\n", k, v)
		default:
			fmt.Fprintf(&b, "%s = %s\n", k, v)
		}
	}
	return b.String()
}

func indexOf(key string) int {
	for i, s := range DBSettings {
		if s.Key == key {
			return i
		}
	}
	return len(DBSettings)
}

// ParseUserDBConf reads back what UserDBConf wrote, so the page shows what is in force. Lines it did not write are ignored.
func ParseUserDBConf(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)
		s, known := settingOf(k)
		if !known {
			continue
		}
		if k == "sql_mode" {
			for word, mode := range sqlModes {
				if mode == v {
					out[k] = word
				}
			}
			continue
		}
		if s.Unit == "M" {
			v = strings.TrimSuffix(v, "M")
		}
		out[k] = v
	}
	return out
}
