package request

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nspcc-dev/neo-go/pkg/encoding/address"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/pkg/errors"
)

type (
	// Param represents a param either passed to
	// the server or to send to a server using
	// the client.
	Param struct {
		Type  paramType
		Value interface{}
	}

	paramType int
	// FuncParam represents a function argument parameter used in the
	// invokefunction RPC method.
	FuncParam struct {
		Type  smartcontract.ParamType `json:"type"`
		Value Param                   `json:"value"`
	}
	// BlockFilter is a wrapper structure for block event filter. The only
	// allowed filter is primary index.
	BlockFilter struct {
		Primary int `json:"primary"`
	}
	// TxFilter is a wrapper structure for transaction event filter. It
	// allows to filter transactions by senders and cosigners.
	TxFilter struct {
		Sender   *util.Uint160 `json:"sender,omitempty"`
		Cosigner *util.Uint160 `json:"cosigner,omitempty"`
	}
	// NotificationFilter is a wrapper structure representing filter used for
	// notifications generated during transaction execution. Notifications can
	// only be filtered by contract hash.
	NotificationFilter struct {
		Contract util.Uint160 `json:"contract"`
	}
	// ExecutionFilter is a wrapper structure used for transaction execution
	// events. It allows to choose failing or successful transactions based
	// on their VM state.
	ExecutionFilter struct {
		State string `json:"state"`
	}
)

// These are parameter types accepted by RPC server.
const (
	defaultT paramType = iota
	StringT
	NumberT
	ArrayT
	FuncParamT
	BlockFilterT
	TxFilterT
	NotificationFilterT
	ExecutionFilterT
)

func (p Param) String() string {
	return fmt.Sprintf("%v", p.Value)
}

// GetString returns string value of the parameter.
func (p Param) GetString() (string, error) {
	str, ok := p.Value.(string)
	if !ok {
		return "", errors.New("not a string")
	}
	return str, nil
}

// GetInt returns int value of te parameter.
func (p Param) GetInt() (int, error) {
	i, ok := p.Value.(int)
	if ok {
		return i, nil
	} else if s, ok := p.Value.(string); ok {
		return strconv.Atoi(s)
	}
	return 0, errors.New("not an integer")
}

// GetArray returns a slice of Params stored in the parameter.
func (p Param) GetArray() ([]Param, error) {
	a, ok := p.Value.([]Param)
	if !ok {
		return nil, errors.New("not an array")
	}
	return a, nil
}

// GetUint256 returns Uint256 value of the parameter.
func (p Param) GetUint256() (util.Uint256, error) {
	s, err := p.GetString()
	if err != nil {
		return util.Uint256{}, err
	}

	return util.Uint256DecodeStringLE(s)
}

// GetUint160FromHex returns Uint160 value of the parameter encoded in hex.
func (p Param) GetUint160FromHex() (util.Uint160, error) {
	s, err := p.GetString()
	if err != nil {
		return util.Uint160{}, err
	}
	if len(s) == 2*util.Uint160Size+2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}

	return util.Uint160DecodeStringLE(s)
}

// GetUint160FromAddress returns Uint160 value of the parameter that was
// supplied as an address.
func (p Param) GetUint160FromAddress() (util.Uint160, error) {
	s, err := p.GetString()
	if err != nil {
		return util.Uint160{}, err
	}

	return address.StringToUint160(s)
}

// GetFuncParam returns current parameter as a function call parameter.
func (p Param) GetFuncParam() (FuncParam, error) {
	fp, ok := p.Value.(FuncParam)
	if !ok {
		return FuncParam{}, errors.New("not a function parameter")
	}
	return fp, nil
}

// GetBytesHex returns []byte value of the parameter if
// it is a hex-encoded string.
func (p Param) GetBytesHex() ([]byte, error) {
	s, err := p.GetString()
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(s)
}

// GetUint160Slice returns a slice of Uint160 stored in the parameter.
func (p Param) GetUint160Slice() ([]util.Uint160, error) {
	array, err := p.GetArray()
	if err != nil {
		return nil, err
	}
	result := make([]util.Uint160, len(array))
	for i, u := range array {
		result[i], err = u.GetUint160FromHex()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (p *Param) UnmarshalJSON(data []byte) error {
	var s string
	var num float64
	// To unmarshal correctly we need to pass pointers into the decoder.
	var attempts = [...]Param{
		{NumberT, &num},
		{StringT, &s},
		{FuncParamT, &FuncParam{}},
		{BlockFilterT, &BlockFilter{}},
		{TxFilterT, &TxFilter{}},
		{NotificationFilterT, &NotificationFilter{}},
		{ExecutionFilterT, &ExecutionFilter{}},
		{ArrayT, &[]Param{}},
	}

	for _, cur := range attempts {
		r := bytes.NewReader(data)
		jd := json.NewDecoder(r)
		jd.DisallowUnknownFields()
		if err := jd.Decode(cur.Value); err == nil {
			p.Type = cur.Type
			// But we need to store actual values, not pointers.
			switch val := cur.Value.(type) {
			case *float64:
				p.Value = int(*val)
			case *string:
				p.Value = *val
			case *FuncParam:
				p.Value = *val
			case *BlockFilter:
				p.Value = *val
			case *TxFilter:
				p.Value = *val
			case *NotificationFilter:
				p.Value = *val
			case *ExecutionFilter:
				if (*val).State == "HALT" || (*val).State == "FAULT" {
					p.Value = *val
				} else {
					continue
				}
			case *[]Param:
				p.Value = *val
			}
			return nil
		}
	}

	return errors.New("unknown type")
}
