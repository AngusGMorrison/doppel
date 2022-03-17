// Package templatecache provides a concurrent, non-blocking cache of composable
// templates.
package templatecache

type Cache struct {
	schematic CacheSchema
}
