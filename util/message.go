package util

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// message errors
var (
	ErrMalformedMessage = errors.New("malformed message")
)

type Message struct {
	CheckType string
	Owner     string
	Repo      string
	CommitSHA string

	Branch string
	PRNum  int
}

func (m Message) Repository() string {
	return m.Owner + "/" + m.Repo
}

func (m Message) Prefix() string {
	if m.CheckType != "tree" {
		return fmt.Sprintf("%s/pull/%d/commits/", m.Repository(), m.PRNum)
	}
	return ""
}

func (m Message) String() string {
	s := fmt.Sprintf("%s/%s/", m.Repository(), m.CheckType)
	if m.CheckType == "tree" {
		s += m.Branch
	} else {
		s += strconv.Itoa(m.PRNum)
	}
	return s
}

// ParseMessage parse message
func ParseMessage(message string) (*Message, error) {
	s := strings.Split(message, "/")
	if len(s) != 6 || (s[2] != "pull" && s[2] != "tree") || s[4] != "commits" {
		return nil, ErrMalformedMessage
	}
	var m Message
	var pull string
	m.CheckType = s[2]
	m.Owner = s[0]
	m.Repo = s[1]
	pull, m.CommitSHA = s[3], s[5]
	prNum := 0
	if m.CheckType == "tree" {
		// branchs
		m.Branch = pull
	} else {
		// pulls
		var err error
		prNum, err = strconv.Atoi(pull)
		if err != nil {
			return nil, ErrMalformedMessage
		}
		m.PRNum = prNum
	}
	return &m, nil
}
