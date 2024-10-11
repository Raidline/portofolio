package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const headerSize = 12

type ErrorSection string

const (
	HeaderSection   ErrorSection = "HEADER"
	RequestSection  ErrorSection = "REQUEST"
	ResponseSection ErrorSection = "RESPONSE"
)

type DnsMessage struct {
	header    dnsHeader
	questions []dnsQuestion
	answers   []dnsAnswer
}

func (m *DnsMessage) Transform(each func(q dnsQuestion, a dnsAnswer) (dnsQuestion, dnsAnswer)) func() bool {
	curr := 0
	return func() bool {
		if curr < len(m.answers) && curr < len(m.questions) {
			question := m.questions[curr]
			answer := m.answers[curr]

			tQ, tA := each(question, answer)

			m.questions[curr] = tQ
			m.answers[curr] = tA

			curr++

			return true
		}

		return false
	}
}

type dnsAnswer struct {
	Name     string
	Type     uint16 // 2 byte -> 16 bits
	Class    uint16 // 2 byte -> 16 bits
	TTL      uint32 // 4 byte -> 32 bits
	RdLength uint16 // 2 byte
	Rdata    []byte // variable (ip address)
}

func unmarshallAnswer(qs []dnsQuestion) []dnsAnswer {
	var answers []dnsAnswer

	for _, q := range qs {
		// we would need to know the size we read, in order to advance for the answer
		answer := dnsAnswer{
			Name:     q.Name, // for now mimic
			Type:     1,      //corresponds to the "A" record type
			Class:    1,      // corresponds to the "IN" record class
			TTL:      60,
			RdLength: 0, //Length of the Rdata
			Rdata:    []byte{},
		}

		answers = append(answers, answer)
	}

	return answers
}

func (a dnsAnswer) marshall() []byte {
	var answerBytes []byte

	domain := encodeDomain(a.Name)

	answerBytes = append(answerBytes, domain...)
	answerBytes = binary.BigEndian.AppendUint16(answerBytes, a.Type)
	answerBytes = binary.BigEndian.AppendUint16(answerBytes, a.Class)
	answerBytes = binary.BigEndian.AppendUint32(answerBytes, a.TTL)
	answerBytes = binary.BigEndian.AppendUint16(answerBytes, a.RdLength)
	answerBytes = append(answerBytes, a.Rdata...)

	return answerBytes
}

type dnsQuestion struct {
	Name  string
	Type  uint16 // 2 byte -> 16 bits
	Class uint16 // 2 byte -> 16 bits
}

func (q dnsQuestion) marshall() []byte {
	var questionBytes []byte
	questionBytes = append(questionBytes, encodeDomain(q.Name)...)
	questionBytes = binary.BigEndian.AppendUint16(questionBytes, q.Type)
	questionBytes = binary.BigEndian.AppendUint16(questionBytes, q.Class)

	return questionBytes
}

func unmarshallQuestion(data []byte) ([]dnsQuestion, error) {

	var questions []dnsQuestion

	decompressedData := decompressQuestions(data)

	nullIndexes := findNullIndexes(decompressedData)
	for i := 0; i < len(nullIndexes); i++ {
		nullIndex := nullIndexes[i] + 4
		if i == 0 {
			questions = append(questions, deserializeQuestion(decompressedData[0:nullIndex+1]))
		} else {
			questions = append(questions, deserializeQuestion(decompressedData[nullIndexes[i-1]+1+4:nullIndex+1]))
		}
	}

	return questions, nil
}

func deserializeQuestion(data []byte) dnsQuestion {
	buf := bytes.NewBuffer(data)
	question := dnsQuestion{}
	name, nullIndex := parseName(data)
	question.Name = name
	buf.Next(nullIndex)
	binary.Read(buf, binary.BigEndian, &question.Type)
	binary.Read(buf, binary.BigEndian, &question.Class)
	return question
}

func findNullIndexes(data []byte) []int {
	var nullIndexes []int
	for i := 0; i < len(data); i++ {
		if data[i] == 0 && data[i+1] == 0 {
			nullIndexes = append(nullIndexes, i)
		}
	}
	return nullIndexes
}

func decompressQuestions(data []byte) []byte {
	decompressedData := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] != 0xC0 {
			decompressedData = append(decompressedData, data[i])
		} else {
			pointer := binary.BigEndian.Uint16(data[i : i+2])
			pointerValue := int(pointer & 0x3FFF)
			pointerValue -= headerSize
			for j := pointerValue; j < len(data); j++ {
				if data[j] == 0x00 {
					decompressedData = append(decompressedData, data[j])
					i += 1
					break
				}
				decompressedData = append(decompressedData, data[j])
			}
		}
	}
	return decompressedData
}

func parseName(data []byte) (string, int) {
	nullIndex := bytes.Index(data, []byte{0})
	var parts []string
	for i := 0; i < nullIndex; i++ {
		length := int(data[i])
		parts = append(parts, string(data[i+1:i+1+length]))
		i += length
	}
	return strings.Join(parts, "."), nullIndex + 1
}

// DnsHeader spec, which is 12 bytes long.
// Packet Identifier (ID)	16 bits	A random ID assigned to query packets. Response packets must reply with the same ID.
// Query/Response Indicator (QR)	1 bit	1 for a reply packet, 0 for a question packet.
// Operation Code (OPCODE)	4 bits	Specifies the kind of query in a message.
// Authoritative Answer (AA)	1 bit	1 if the responding server "owns" the domain queried, i.e., it's authoritative.
// Truncation (TC)	1 bit	1 if the message is larger than 512 bytes. Always 0 in UDP responses.
// Recursion Desired (RD)	1 bit	Sender sets this to 1 if the server should recursively resolve this query, 0 otherwise.
// Recursion Available (RA)	1 bit	Server sets this to 1 to indicate that recursion is available.
// Reserved (Z)	3 bits	Used by DNSSEC queries. At inception, it was reserved for future use.
// Response Code (RCODE)	4 bits	Response code indicating the status of the response.
// Question Count (QDCOUNT)	16 bits	Number of questions in the Question section.
// Answer Record Count (ANCOUNT)	16 bits	Number of records in the Answer section.
// Authority Record Count (NSCOUNT)	16 bits	Number of records in the Authority section.
// Additional Record Count (ARCOUNT)	16 bits	Number of records in the Additional section.
type dnsHeader struct {
	Id           uint16 // 16 bits
	flags        headerFlags
	Qcount       uint16 // 16 bits
	ARecCount    uint16 // 16 bits
	AuthRecCount uint16 // 16 bits
	AddRecCount  uint16 // 16 bits
}

type headerFlags struct {
	QueryResInd bool  // 1 bit
	OpCode      uint8 // 4 bit
	AuthAns     bool  // 1 bit
	Tc          bool  // 1 bit -> truncation
	RecrDes     bool  // 1 bit -> recursion desired
	RecrAvl     bool  // 1 bit -> recursion available
	Reserved    uint8 // 3bit (Z)
	ResCode     uint8 //4 bits
}

func unmarshallHeader(header []byte) (dnsHeader, error) {
	return dnsHeader{
		Id:           binary.BigEndian.Uint16(header[:2]), // 16 bits
		flags:        unmarshallFlags(header[2:4]),        // 16 bits
		Qcount:       0,                                   // 16 bits
		ARecCount:    0,                                   // 16 bits
		AuthRecCount: 0,                                   // 16 bits
		AddRecCount:  0,                                   // 16 bits
	}, nil
}

func (h dnsHeader) marshall() []byte {
	response := make([]byte, 12)

	binary.BigEndian.PutUint16(response[0:2], h.Id)
	copy(response[2:4], h.flags.marshall())
	binary.BigEndian.PutUint16(response[4:6], h.Qcount)
	binary.BigEndian.PutUint16(response[6:8], h.ARecCount)
	binary.BigEndian.PutUint16(response[8:10], h.AuthRecCount)
	binary.BigEndian.PutUint16(response[10:12], h.AddRecCount)

	return response
}

func (f *headerFlags) marshall() []byte {
	first := byte(0)
	if f.QueryResInd {
		first |= 1 << 7
	}
	first |= f.OpCode << 3
	if f.AuthAns {
		first |= 1 << 2
	}
	if f.Tc {
		first |= 1 << 1
	}
	if f.RecrDes {
		first |= 1
	}
	second := byte(0)
	if f.RecrAvl {
		second |= 1 << 7
	}
	second |= f.ResCode << 4
	second |= f.ResCode
	return []byte{first, second}
}

func unmarshallFlags(b []byte) headerFlags {
	first := b[0]
	second := b[1]
	opcode := (first & 120) >> 3
	var rcode uint8
	if opcode != 0 {
		rcode = 4
	}
	return headerFlags{
		QueryResInd: true,
		OpCode:      opcode,
		AuthAns:     (first & 4) == 1,
		Tc:          (first & 2) == 1,
		RecrDes:     (first & 1) == 1,
		RecrAvl:     (second & 128) == 1,
		Reserved:    (second & 112) >> 4,
		ResCode:     rcode,
	}
}

func Unmarshall(data []byte, size int) (*DnsMessage, error) {
	if size <= 12 {
		return nil, &ParseError{
			Section:  HeaderSection,
			Index:    0,
			Property: "Header Length",
			Reason:   fmt.Sprintf("Message should have at lease size of 12, but it's instead %d", size),
		}
	}

	// unmarshallHeader the header
	headerParsed, errHeader := unmarshallHeader(data[:12])

	if errHeader != nil {
		fmt.Printf("Error while parsing :: %s", errHeader.Error())

		return nil, errHeader
	}

	questions, qErr := unmarshallQuestion(data[12:size])

	if qErr != nil {
		fmt.Printf("Error while parsing :: %s", qErr.Error())

		return nil, qErr
	}

	answers := unmarshallAnswer(questions)

	headerParsed.Qcount = uint16(len(questions))
	headerParsed.ARecCount = uint16(len(answers))

	fmt.Println("answers len", headerParsed.ARecCount)
	fmt.Println("questions len", headerParsed.Qcount)

	return &DnsMessage{
		header:    headerParsed,
		questions: questions,
		answers:   answers,
	}, nil
}

func (m *DnsMessage) Marshall() ([]byte, error) {

	if m.header.Qcount != uint16(len(m.questions)) {
		return nil, &ParseError{
			Section:  RequestSection,
			Index:    -1,
			Property: "QCOUNT property",
			Reason: fmt.Sprintf("QCount property should have value %d, but is instead %d",
				m.header.Qcount, len(m.questions)),
		}
	}

	if m.header.ARecCount != uint16(len(m.answers)) {
		return nil, &ParseError{
			Section:  ResponseSection,
			Index:    -1,
			Property: "ANCOUNT",
			Reason: fmt.Sprintf("ANCOUNT property should have value %d, but is instead %d",
				m.header.ARecCount, len(m.answers)),
		}
	}

	var response []byte

	response = append(response, m.header.marshall()...)
	for _, question := range m.questions {
		response = append(response, question.marshall()...)
	}
	for _, answer := range m.answers {
		response = append(response, answer.marshall()...)
	}

	return response, nil
}

func encodeDomain(domain string) []byte {
	var r []byte
	for _, s := range strings.Split(domain, ".") {
		r = append(r, byte(len(s)))
		r = append(r, []byte(s)...)
	}
	r = append(r, 0x00)
	return r
}

type ParseError struct {
	Section  ErrorSection
	Index    int
	Property string
	Reason   string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("Error parsing section %s, at index %d, for property %s, with reason %s",
		e.Section, e.Index, e.Property, e.Reason)
}
