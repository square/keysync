// Copyright 2017 Square Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/square/keysync"
	"github.com/stretchr/testify/assert"
)

type fakeEmailSender struct {
	invocations []sendMailInvocation
}

type sendMailInvocation struct {
	addr, from string
	to         []string
	msg        []byte
}

func (fes *fakeEmailSender) sendMail(addr string, from string, to []string, msg []byte) error {
	fes.invocations = append(fes.invocations, sendMailInvocation{
		addr: addr,
		from: from,
		to:   to,
		msg:  msg,
	})
	return nil
}

func TestAlertEmail(t *testing.T) {
	hostname, err := os.Hostname()
	assert.Nil(t, err)

	config := keysync.MonitorConfig{
		AlertEmailServer:    "localhost:smtp",
		AlertEmailRecipient: "foo@example.org",
		AlertEmailSender:    "bar@example.org",
	}

	mockSender := &fakeEmailSender{invocations: []sendMailInvocation{}}

	// Should trigger an email
	emailErrors(config, []error{errors.New("test failure")}, mockSender)

	assert.Len(t, mockSender.invocations, 1)

	call := mockSender.invocations[0]
	assert.Equal(t, call.addr, config.AlertEmailServer)
	assert.Equal(t, call.from, config.AlertEmailSender)

	assert.Len(t, call.to, 1)
	assert.Equal(t, call.to[0], config.AlertEmailRecipient)

	assert.Contains(t, string(call.msg), "To: foo@example.org\r\n")
	assert.Contains(t, string(call.msg), "From: bar@example.org\r\n")
	assert.Contains(t, string(call.msg), fmt.Sprintf("Subject: %s\r\n", hostname))
	assert.Contains(t, string(call.msg), "- test failure")
}

func TestAlertEmailDefaults(t *testing.T) {
	hostname, err := os.Hostname()
	assert.Nil(t, err)

	config := keysync.MonitorConfig{
		AlertEmailRecipient: "foo@example.org",
	}

	mockSender := &fakeEmailSender{invocations: []sendMailInvocation{}}

	// Should trigger an email
	emailErrors(config, []error{errors.New("test failure")}, mockSender)

	assert.Len(t, mockSender.invocations, 1)

	call := mockSender.invocations[0]
	expectedFrom := fmt.Sprintf("%s@%s", "keysync-monitor", hostname)

	assert.Equal(t, call.addr, "localhost:25")
	assert.Equal(t, call.from, expectedFrom)

	assert.Len(t, call.to, 1)
	assert.Equal(t, call.to[0], config.AlertEmailRecipient)

	assert.Contains(t, string(call.msg), "To: foo@example.org\r\n")
	assert.Contains(t, string(call.msg), fmt.Sprintf("From: %s\r\n", expectedFrom))
	assert.Contains(t, string(call.msg), fmt.Sprintf("Subject: %s\r\n", hostname))
	assert.Contains(t, string(call.msg), "- test failure")
}

func TestAlertEmailNoRecipient(t *testing.T) {
	config := keysync.MonitorConfig{}
	mockSender := &fakeEmailSender{invocations: []sendMailInvocation{}}

	// Should *not* trigger an email
	emailErrors(config, []error{errors.New("test failure")}, mockSender)

	assert.Len(t, mockSender.invocations, 0)
}
