// Package providers contains per-host Ripper implementations.
// Each file in this package corresponds to one image hosting provider.
//
// All rippers follow the same pattern:
//  1. Fetch the image page HTML via an HTTP GET.
//  2. Extract the direct media URL using a regexp or simple string scan.
//  3. Return the direct URL(s) so the core downloader can stream them to disk.
package providers
