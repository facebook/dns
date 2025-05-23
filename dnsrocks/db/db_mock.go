/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Code generated by MockGen. DO NOT EDIT.
// Source: dns/fbdns/db/db.go

// Package db is a generated GoMock package.
package db

import (
	net "net"
	reflect "reflect"

	dns "github.com/miekg/dns"
	gomock "go.uber.org/mock/gomock"
)

// MockDBI is a mock of DBI interface
type MockDBI struct {
	ctrl     *gomock.Controller
	recorder *MockDBIMockRecorder
}

// MockDBIMockRecorder is the mock recorder for MockDBI
type MockDBIMockRecorder struct {
	mock *MockDBI
}

// NewMockDBI creates a new mock instance
func NewMockDBI(ctrl *gomock.Controller) *MockDBI {
	mock := &MockDBI{ctrl: ctrl}
	mock.recorder = &MockDBIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockDBI) EXPECT() *MockDBIMockRecorder {
	return m.recorder
}

// NewContext mocks base method
func (m *MockDBI) NewContext() Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewContext")
	ret0, _ := ret[0].(Context)
	return ret0
}

// NewContext indicates an expected call of NewContext
func (mr *MockDBIMockRecorder) NewContext() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewContext", reflect.TypeOf((*MockDBI)(nil).NewContext))
}

// Find mocks base method
func (m *MockDBI) Find(key []byte, context Context) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Find", key, context)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Find indicates an expected call of Find
func (mr *MockDBIMockRecorder) Find(key, context interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Find", reflect.TypeOf((*MockDBI)(nil).Find), key, context)
}

// ForEach mocks base method
func (m *MockDBI) ForEach(key []byte, f func([]byte) error, context Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ForEach", key, f, context)
	ret0, _ := ret[0].(error)
	return ret0
}

// ForEach indicates an expected call of ForEach
func (mr *MockDBIMockRecorder) ForEach(key, f, context interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ForEach", reflect.TypeOf((*MockDBI)(nil).ForEach), key, f, context)
}

// FreeContext mocks base method
func (m *MockDBI) FreeContext(arg0 Context) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FreeContext", arg0)
}

// FreeContext indicates an expected call of FreeContext
func (mr *MockDBIMockRecorder) FreeContext(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FreeContext", reflect.TypeOf((*MockDBI)(nil).FreeContext), arg0)
}

// FindMap mocks base method
func (m *MockDBI) FindMap(domain, mtype []byte, context Context) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindMap", domain, mtype, context)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindMap indicates an expected call of FindMap
func (mr *MockDBIMockRecorder) FindMap(domain, mtype, context interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindMap", reflect.TypeOf((*MockDBI)(nil).FindMap), domain, mtype, context)
}

// GetLocationByMap mocks base method
func (m *MockDBI) GetLocationByMap(ipnet *net.IPNet, mapID []byte, context Context) ([]byte, uint8, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLocationByMap", ipnet, mapID, context)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(uint8)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetLocationByMap indicates an expected call of GetLocationByMap
func (mr *MockDBIMockRecorder) GetLocationByMap(ipnet, mapID, context interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLocationByMap", reflect.TypeOf((*MockDBI)(nil).GetLocationByMap), ipnet, mapID, context)
}

// Close mocks base method
func (m *MockDBI) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockDBIMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockDBI)(nil).Close))
}

// Reload mocks base method
func (m *MockDBI) Reload(path string) (DBI, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Reload", path)
	ret0, _ := ret[0].(DBI)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Reload indicates an expected call of Reload
func (mr *MockDBIMockRecorder) Reload(path interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Reload", reflect.TypeOf((*MockDBI)(nil).Reload), path)
}

// GetStats mocks base method
func (m *MockDBI) GetStats() map[string]int64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetStats")
	ret0, _ := ret[0].(map[string]int64)
	return ret0
}

// GetStats indicates an expected call of GetStats
func (mr *MockDBIMockRecorder) GetStats() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetStats", reflect.TypeOf((*MockDBI)(nil).GetStats))
}

// ClosestKeyFinder mocks base method
func (m *MockDBI) ClosestKeyFinder() ClosestKeyFinder {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClosestKeyFinder")
	ret0, _ := ret[0].(ClosestKeyFinder)
	return ret0
}

// ClosestKeyFinder indicates an expected call of ClosestKeyFinder
func (mr *MockDBIMockRecorder) ClosestKeyFinder() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClosestKeyFinder", reflect.TypeOf((*MockDBI)(nil).ClosestKeyFinder))
}

// MockClosestKeyFinder is a mock of ClosestKeyFinder interface
type MockClosestKeyFinder struct {
	ctrl     *gomock.Controller
	recorder *MockClosestKeyFinderMockRecorder
}

// MockClosestKeyFinderMockRecorder is the mock recorder for MockClosestKeyFinder
type MockClosestKeyFinderMockRecorder struct {
	mock *MockClosestKeyFinder
}

// NewMockClosestKeyFinder creates a new mock instance
func NewMockClosestKeyFinder(ctrl *gomock.Controller) *MockClosestKeyFinder {
	mock := &MockClosestKeyFinder{ctrl: ctrl}
	mock.recorder = &MockClosestKeyFinderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockClosestKeyFinder) EXPECT() *MockClosestKeyFinderMockRecorder {
	return m.recorder
}

// FindClosestKey mocks base method
func (m *MockClosestKeyFinder) FindClosestKey(key []byte, context Context) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindClosestKey", key, context)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindClosestKey indicates an expected call of FindClosestKey
func (mr *MockClosestKeyFinderMockRecorder) FindClosestKey(key, context interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindClosestKey", reflect.TypeOf((*MockClosestKeyFinder)(nil).FindClosestKey), key, context)
}

// MockContext is a mock of Context interface
type MockContext struct {
	ctrl     *gomock.Controller
	recorder *MockContextMockRecorder
}

// MockContextMockRecorder is the mock recorder for MockContext
type MockContextMockRecorder struct {
	mock *MockContext
}

// NewMockContext creates a new mock instance
func NewMockContext(ctrl *gomock.Controller) *MockContext {
	mock := &MockContext{ctrl: ctrl}
	mock.recorder = &MockContextMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockContext) EXPECT() *MockContextMockRecorder {
	return m.recorder
}

// Reset mocks base method
func (m *MockContext) Reset() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Reset")
}

// Reset indicates an expected call of Reset
func (mr *MockContextMockRecorder) Reset() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Reset", reflect.TypeOf((*MockContext)(nil).Reset))
}

// MockReader is a mock of Reader interface
type MockReader struct {
	ctrl     *gomock.Controller
	recorder *MockReaderMockRecorder
}

// MockReaderMockRecorder is the mock recorder for MockReader
type MockReaderMockRecorder struct {
	mock *MockReader
}

// NewMockReader creates a new mock instance
func NewMockReader(ctrl *gomock.Controller) *MockReader {
	mock := &MockReader{ctrl: ctrl}
	mock.recorder = &MockReaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockReader) EXPECT() *MockReaderMockRecorder {
	return m.recorder
}

// FindLocation mocks base method
func (m_2 *MockReader) FindLocation(qname []byte, m *dns.Msg, ip string) (*dns.EDNS0_SUBNET, *Location, error) {
	m_2.ctrl.T.Helper()
	ret := m_2.ctrl.Call(m_2, "FindLocation", qname, m, ip)
	ret0, _ := ret[0].(*dns.EDNS0_SUBNET)
	ret1, _ := ret[1].(*Location)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// FindLocation indicates an expected call of FindLocation
func (mr *MockReaderMockRecorder) FindLocation(qname, m, ip interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindLocation", reflect.TypeOf((*MockReader)(nil).FindLocation), qname, m, ip)
}

// IsAuthoritative mocks base method
func (m *MockReader) IsAuthoritative(q []byte, loc *Location) (bool, bool, []byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsAuthoritative", q, loc)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].([]byte)
	ret3, _ := ret[3].(error)
	return ret0, ret1, ret2, ret3
}

// IsAuthoritative indicates an expected call of IsAuthoritative
func (mr *MockReaderMockRecorder) IsAuthoritative(q, loc interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsAuthoritative", reflect.TypeOf((*MockReader)(nil).IsAuthoritative), q, loc)
}

// FindAnswer mocks base method
func (m *MockReader) FindAnswer(q, packedControlName []byte, qname string, qtype uint16, loc Location, a *dns.Msg, maxAnswer int) (bool, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindAnswer", q, packedControlName, qname, qtype, loc, a, maxAnswer)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// FindAnswer indicates an expected call of FindAnswer
func (mr *MockReaderMockRecorder) FindAnswer(q, packedControlName, qname, qtype, loc, a, maxAnswer interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindAnswer", reflect.TypeOf((*MockReader)(nil).FindAnswer), q, packedControlName, qname, qtype, loc, a, maxAnswer)
}

// EcsLocation mocks base method
func (m *MockReader) EcsLocation(q []byte, ecs *dns.EDNS0_SUBNET) (*Location, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EcsLocation", q, ecs)
	ret0, _ := ret[0].(*Location)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EcsLocation indicates an expected call of EcsLocation
func (mr *MockReaderMockRecorder) EcsLocation(q, ecs interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EcsLocation", reflect.TypeOf((*MockReader)(nil).EcsLocation), q, ecs)
}

// ResolverLocation mocks base method
func (m *MockReader) ResolverLocation(q []byte, ip string) (*Location, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolverLocation", q, ip)
	ret0, _ := ret[0].(*Location)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ResolverLocation indicates an expected call of ResolverLocation
func (mr *MockReaderMockRecorder) ResolverLocation(q, ip interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolverLocation", reflect.TypeOf((*MockReader)(nil).ResolverLocation), q, ip)
}

// findLocation mocks base method
func (m *MockReader) findLocation(q, mtype []byte, ipnet *net.IPNet) (*Location, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "findLocation", q, mtype, ipnet)
	ret0, _ := ret[0].(*Location)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// findLocation indicates an expected call of findLocation
func (mr *MockReaderMockRecorder) findLocation(q, mtype, ipnet interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "findLocation", reflect.TypeOf((*MockReader)(nil).findLocation), q, mtype, ipnet)
}

// ForEach mocks base method
func (m *MockReader) ForEach(key []byte, f func([]byte) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ForEach", key, f)
	ret0, _ := ret[0].(error)
	return ret0
}

// ForEach indicates an expected call of ForEach
func (mr *MockReaderMockRecorder) ForEach(key, f interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ForEach", reflect.TypeOf((*MockReader)(nil).ForEach), key, f)
}

// ForEachResourceRecord mocks base method
func (m *MockReader) ForEachResourceRecord(domainName []byte, loc *Location, parseRecord func([]byte) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ForEachResourceRecord", domainName, loc, parseRecord)
	ret0, _ := ret[0].(error)
	return ret0
}

// ForEachResourceRecord indicates an expected call of ForEachResourceRecord
func (mr *MockReaderMockRecorder) ForEachResourceRecord(domainName, loc, parseRecord interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ForEachResourceRecord", reflect.TypeOf((*MockReader)(nil).ForEachResourceRecord), domainName, loc, parseRecord)
}

// Close mocks base method
func (m *MockReader) Close() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Close")
}

// Close indicates an expected call of Close
func (mr *MockReaderMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockReader)(nil).Close))
}
