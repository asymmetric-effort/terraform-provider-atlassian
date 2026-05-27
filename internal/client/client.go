// Package client provides the Atlassian Cloud API client.
//
// This is a purpose-built thin client with zero third-party Atlassian SDK
// dependencies. It handles pagination transparency, rate-limit resilience
// with exponential backoff and jitter, and error translation to clear
// user-facing messages.
package client
