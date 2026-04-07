package validate

import (
	"fmt"
	"log"
)

// DedupeMajorNpcNamesBySuffix ensures majorNPCs[].name are unique; appends " (2)", " (3)" on collision.
func DedupeMajorNpcNamesBySuffix(list []interface{}) []interface{} {
	if list == nil {
		return list
	}
	seen := map[string]int{}
	out := make([]interface{}, 0, len(list))
	for idx, item := range list {
		row, ok := item.(map[string]interface{})
		if !ok || row == nil {
			out = append(out, item)
			continue
		}
		name := str(row["name"])
		if name == "" {
			out = append(out, item)
			continue
		}
		key := normalizeNameKey(name)
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, has := seen[key]; !has {
			seen[key] = 1
			out = append(out, item)
			continue
		}
		n := seen[key] + 1
		seen[key] = n
		dis := fmt.Sprintf("%s (%d)", name, n)
		log.Printf("[campaign] majorNPCs[%d] renamed duplicate %q -> %q", idx, name, dis)
		next := shallowClone(row)
		next["name"] = dis
		out = append(out, next)
	}
	return out
}
