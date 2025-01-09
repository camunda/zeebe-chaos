package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/camunda/zeebe-chaos/go-chaos/cmd"
)

func Test_ExecuteRootCmd(t *testing.T) {
	// given
	expectedString := `A chaos experimenting toolkit for Zeebe.
    Perfect to inject some chaos into your brokers and gateways.`
	rootCmd := cmd.NewCmd()
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)

	// when
	err := rootCmd.Execute()
	require.NoError(t, err)

	// then
	out, err := io.ReadAll(b)
	require.NoError(t, err)
	actual := string(out)
	assert.Contains(t, actual, expectedString, "Doesn't contain expected string")
}
