# doppel

A concurrent, non-blocking, compositing cache for Go templates.

doppel provides a simple, thread-safe way to compose and cache nested templates as they're required. Rather than parsing sub-templates from scratch each time they're needed, a Doppel instance compiles a named combination of templates the first time it's requested and stores it in memory. Once parsed, retrieval is on the order of nanoseconds.

Common sub-templates (e.g. a recurrent nav bar) are parsed once and shared between compositions, reducing both the Doppel's memory footprint and the number of parsing operations required.

Each Get request to the Doppel is non-blocking. Even where a template must be parsed for the first time, concurrent requests for other templates proceed freely.

**Package documentation**: https://godoc.org/github.com/AngusGMorrison/doppel

## Schematics
At the core of doppel are `CacheSchematics` and `TemplateSchematics`. A `CacheSchematic` is an acyclic graph of named `TemplateSchematic`s that collectively describe how to build a complete template from component parts.

For example, template `homepage` may contain unique `content` and `sidebar` subtemplates, but also depend on a common `nav` which itself depends on a universal `base` template. Here's what the `CacheSchematic` looks like:

```Go
schematic := CacheSchematic{
  "base": {"", []string{"path/to/base"},
  "nav": {"base", []string{"path/to/nav"},
  "homepage": {"nav", []string{"path/to/homepage", "path/to/content", "path/to/sidebar"},
}
```

When `homepage` is requested for the first time, it requests the `nav` template from the cache. The first time `nav` is requested, it will request `base` from the cache. However, if `nav` has previously been requested, its cached value is a composition of `nav` and `base`, so `base` does not require a lookup.

With the `nav` template retrieved, `homepage` is parsed from a combination of `nav` and the subtemplates unique to `homepage`, given as a slice of strings. The completed `homepage` template is then cached eliminating the parsing phase the next time it is requested.

Each `CacheSchematic` is checked for cycles before use.

## Package-level and local Doppels
For convenience, doppel provides a package-level cache, instantiated with `Initialize(cs CacheSchematic, ...opts CacheOption)`, along with the functions `Get(ctx context.Context, name string)`, `Shutdown(gracePeriod time.Duration)` and `Close()` to perform operations on it.

New Doppels can be instantiated with `New(cs CacheSchematic, ...opts CacheOption)`, which returns a `*Doppel` with a live cache or an error. The same operations are available to these local Doppels.

## CacheOptions
Various functional options are available for customizing the cache:
* `WithGlobalTimeout`: enforce a time limit for all requests to the cache.
* `WithLogger`: provide a logger for insight into each request's status.
* `WithRetryTimeouts`: specify that parsing should be reattempted for cache entries with errors resulting from request `context` cancellations or timeouts.
