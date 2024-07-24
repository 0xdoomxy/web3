package encoder

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"reflect"
	"strings"
)

// RLP 以节省空间的格式标准化节点之间的数据传输。RLP 的目的是对任意嵌套的二进制数据数组进行编码，
// RLP 是以太坊执行层中用于序列化对象的主要编码方法。RLP 的主要目的是对结构进行编码；除了正整数之外，
// RLP 将特定数据类型（例如字符串、浮点数）的编码委托给高阶协议。正整数必须以大端二进制形式表示，且没有前导零（从而使整数值零等同于空字节数组）。
// 任何使用 RLP 的高阶协议都必须将具有前导零的反序列化正整数视为无效。

func RLPEncode(item any) (res []byte) {
	// RLP编码定义如下：
	// 如果列表的总有效负载（即所有 RLP 编码项的总长度）为 0-55 字节长，则 RLP 编码由值为0xc0 的单个字节加上有效负载的长度，然后是项的 RLP 编码的连接。因此，第一个字节的范围为[0xc0, 0xf7](dec. [192, 247])。
	// 如果列表的总有效负载长度超过 55 个字节，则 RLP 编码由一个值为0xf7 的字节加上二进制形式的有效负载长度（以字节为单位），后跟有效负载的长度，后跟项目的 RLP 编码的连接组成。因此，第一个字节的范围是[0xf8, 0xff](dec. [248, 255])。
	switch item.(type) {
	case uint, uint8, uint16, uint32, uint64:
		//如果输入为非值（uint(0)、[]byte{}、string(“”)、空指针……），则 RLP 编码为0x80。请注意，0x00值字节不是非值
		var itemUint = item.(uint)
		if itemUint == 0 {
			res = []byte{0x80}
			return
		}
		// 对于正整数，则先将其转换为大端解释为整数的最短字节数组，然后按照下面的规则编码为字符串。
		var itemEncode = newBinaryUint(uint64(itemUint))
		return RLPEncode(itemEncode)
	case []byte:
		var itemByt = item.([]byte)
		// 如果字符串长度为 2^64 字节或更长，则可能无法编码。
		if uint64(len(itemByt)) > math.MaxUint64 {
			panic("The length of the byte array is too long")
		}
		//如果输入为非值（uint(0)、[]byte{}、string(“”)、空指针……），则 RLP 编码为0x80。请注意，0x00值字节不是非值
		if len(itemByt) <= 0 {
			res = []byte{0x80}
			return
		}
		// [0x00, 0x7f]对于值在（十进制）范围内的单个字节[0, 127]，该字节是其自己的 RLP 编码。
		if len(itemByt) == 1 && itemByt[0] < 0x7f {
			res = item.([]byte)
			return
		}
		res = make([]byte, 0)
		// 否则，如果字符串长度为 0-55 字节，则 RLP 编码由值为0x80（十进制 128）的单个字节加上字符串的长度以及字符串后跟的字符串组成。
		// 因此，第一个字节的范围为[0x80, 0xb7]（十进制[128, 183]）。
		if len(itemByt) < 56 {
			res = append(res, byte(0x80+len(itemByt)))
			res = append(res, itemByt...)
			return
		}
		// 如果字符串长度超过 55 个字节，RLP 编码将由一个值为0xb7（十进制 183）的字节加上二进制形式的字符串长度（以字节为单位），
		//后跟字符串的长度，后跟字符串。例如，1024 字节长的字符串将被编码为\xb9\x04\x00（十进制185, 4, 0），
		//后跟字符串。这里，0xb9（183 + 2 = 185）作为第一个字节，后跟 2 个字节0x0400（十进制 1024），表示实际字符串的长度。
		//因此，第一个字节的范围是[0xb8, 0xbf]（十进制[184, 191]）。
		var itemLengthCode = newBinaryUint(uint64(len(itemByt)))
		res = append(res, byte(0xb7+len(itemLengthCode)))
		res = append(res, itemLengthCode...)
		res = append(res, itemByt...)
	case string:
		var itemStr = item.(string)
		if strings.HasPrefix(itemStr, "0x") {
			// Remove the "0x" prefix and decode the hex string into a byte slice.
			hexBytes, err := hex.DecodeString(itemStr[2:])
			if err != nil {
				panic("Invalid hex string")
			}
			return RLPEncode(hexBytes)
		} else {
			return RLPEncode([]byte(itemStr))
		}
	case bool:
		var itemBool = item.(bool)
		switch itemBool {
		case true:
			res = []byte{0x01}
		case false:
			res = []byte{0x80}
		}
	case any:
		var obsure = item.(any)
		if obsure == nil {
			res = []byte{0x80}
			return
		}
		typ := reflect.TypeOf(obsure).Kind()
		if typ == reflect.Ptr {
			obsure = reflect.ValueOf(obsure).Elem().Interface()
			typ = reflect.TypeOf(obsure).Kind()
		}
		var reflectV = reflect.ValueOf(obsure)
		switch typ {
		case reflect.Array, reflect.Slice:
			var reflectVLen = reflectV.Len()
			if reflectVLen <= 0 {
				res = []byte{0xc0}
				return
			}
			tmp := make([]byte, 0)
			var sub reflect.Value
			for i := 0; i < reflectVLen; i++ {
				sub = reflectV.Index(i)
				if sub.CanInterface() {
					tmp = append(tmp, RLPEncode(sub.Interface())...)
				}
			}
			if len(tmp) < 55 {
				res = append(res, byte(0xc0+len(tmp)))
				res = append(res, tmp...)
				return
			}
			var tmpLengthCode = newBinaryUint(uint64(len(tmp)))
			res = append(res, byte(0xf7+len(tmpLengthCode)))
			res = append(res, tmpLengthCode...)
			res = append(res, tmp...)
			return
		case reflect.Struct:
			var sub reflect.Value
			for i := 0; i < reflectV.NumField(); i++ {
				sub = reflectV.Field(i)
				if sub.CanInterface() {
					res = append(res, RLPEncode(sub.Interface())...)
				}
			}
		}
	default:
	}
	return
}
func newBinaryUint(item uint64) []byte {
	var itemEncode = make([]byte, 8)
	binary.BigEndian.PutUint64(itemEncode, uint64(item))
	for i := len(itemEncode) - 1; i >= 0; i-- {
		if itemEncode[i] == 0 {
			if i == len(itemEncode)-1 {
				return []byte{}
			}
			itemEncode = itemEncode[i+1:]
			break
		}
	}
	return itemEncode
}
