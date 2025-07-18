package testmanagerd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/danielpaulus/go-ios/ios/appservice"

	"github.com/danielpaulus/go-ios/ios/house_arrest"

	"github.com/danielpaulus/go-ios/ios"
	dtx "github.com/danielpaulus/go-ios/ios/dtx_codec"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/nskeyedarchiver"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type XCTestManager_IDEInterface struct {
	IDEDaemonProxy         *dtx.Channel
	testBundleReadyChannel chan dtx.Message
	testConfig             nskeyedarchiver.XCTestConfiguration
}
type XCTestManager_DaemonConnectionInterface struct {
	IDEDaemonProxy *dtx.Channel
}

func (xide XCTestManager_IDEInterface) testBundleReady() (uint64, uint64) {
	msg := <-xide.testBundleReadyChannel
	protocolVersion, _ := nskeyedarchiver.Unarchive(msg.Auxiliary.GetArguments()[0].([]byte))
	minimalVersion, _ := nskeyedarchiver.Unarchive(msg.Auxiliary.GetArguments()[1].([]byte))
	return protocolVersion[0].(uint64), minimalVersion[0].(uint64)
}

func testRunnerReadyWithCapabilitiesConfig(testConfig nskeyedarchiver.XCTestConfiguration) dtx.MethodWithResponse {
	return func(msg dtx.Message) (interface{}, error) {
		// protocolVersion, _ := nskeyedarchiver.Unarchive(msg.Auxiliary.GetArguments()[0].([]byte))
		// nskeyedarchiver.XCTCapabilities
		response := testConfig
		// caps := protocolVersion[0].(nskeyedarchiver.XCTCapabilities)

		return response, nil
	}
}

func (xdc XCTestManager_DaemonConnectionInterface) startExecutingTestPlanWithProtocolVersion(channel *dtx.Channel, version uint64) error {
	return channel.MethodCallAsync("_IDE_startExecutingTestPlanWithProtocolVersion:", version)
}

func (xdc XCTestManager_DaemonConnectionInterface) authorizeTestSessionWithProcessID(pid uint64) (bool, error) {
	rply, err := xdc.IDEDaemonProxy.MethodCall("_IDE_authorizeTestSessionWithProcessID:", pid)
	if err != nil {
		log.Errorf("authorizeTestSessionWithProcessID failed: %v, err:%v", pid, err)
		return false, err
	}
	returnValue := rply.Payload[0]
	var val bool
	var ok bool
	if val, ok = returnValue.(bool); !ok {
		return val, fmt.Errorf("_IDE_authorizeTestSessionWithProcessID: got wrong returnvalue: %s", rply.Payload)
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("_IDE_authorizeTestSessionWithProcessID: reply")

	return val, err
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateSessionWithIdentifierAndCaps(uuid uuid.UUID, caps nskeyedarchiver.XCTCapabilities) (nskeyedarchiver.XCTCapabilities, error) {
	var val nskeyedarchiver.XCTCapabilities
	var ok bool
	rply, err := xdc.IDEDaemonProxy.MethodCall("_IDE_initiateSessionWithIdentifier:capabilities:", nskeyedarchiver.NewNSUUID(uuid), caps)
	if err != nil {
		log.Errorf("initiateSessionWithIdentifierAndCaps failed: %v", err)
		return val, err
	}
	returnValue := rply.Payload[0]
	if val, ok = returnValue.(nskeyedarchiver.XCTCapabilities); !ok {
		return val, fmt.Errorf("_IDE_initiateSessionWithIdentifier:capabilities: got wrong returnvalue: %s", rply.Payload)
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("_IDE_initiateSessionWithIdentifier:capabilities: reply")

	return val, err
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateControlSessionWithCapabilities(caps nskeyedarchiver.XCTCapabilities) (nskeyedarchiver.XCTCapabilities, error) {
	var val nskeyedarchiver.XCTCapabilities
	var ok bool
	rply, err := xdc.IDEDaemonProxy.MethodCall("_IDE_initiateControlSessionWithCapabilities:", caps)
	if err != nil {
		log.Errorf("initiateControlSessionWithCapabilities failed: %v", err)
		return val, err
	}
	returnValue := rply.Payload[0]

	if val, ok = returnValue.(nskeyedarchiver.XCTCapabilities); !ok {
		return val, fmt.Errorf("_IDE_initiateControlSessionWithCapabilities got wrong returnvalue: %s", rply.Payload)
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("_IDE_initiateControlSessionWithCapabilities reply")

	return val, err
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateSessionWithIdentifier(sessionIdentifier uuid.UUID, protocolVersion uint64) (uint64, error) {
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName}).Debug("Launching init test Session")
	var val uint64
	var ok bool
	rply, err := xdc.IDEDaemonProxy.MethodCall(
		"_IDE_initiateSessionWithIdentifier:forClient:atPath:protocolVersion:",
		nskeyedarchiver.NewNSUUID(sessionIdentifier),
		"thephonedoesntcarewhatisendhereitseems",
		"/Applications/Xcode.app",
		protocolVersion)
	if err != nil {
		log.Errorf("initiateSessionWithIdentifier failed: %v", err)
		return val, err
	}
	returnValue := rply.Payload[0]
	if val, ok = returnValue.(uint64); !ok {
		return 0, fmt.Errorf("initiateSessionWithIdentifier got wrong returnvalue: %s", rply.Payload)
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("init test session reply")

	return val, err
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateControlSessionForTestProcessID(pid uint64, protocolVersion uint64) error {
	rply, err := xdc.IDEDaemonProxy.MethodCall("_IDE_initiateControlSessionForTestProcessID:protocolVersion:", pid, protocolVersion)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("initiateControlSessionForTestProcessID reply")
	return nil
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateControlSessionWithProtocolVersion(protocolVersion uint64) (uint64, error) {
	rply, err := xdc.IDEDaemonProxy.MethodCall("_IDE_initiateControlSessionWithProtocolVersion:", protocolVersion)
	if err != nil {
		return 0, err
	}
	returnValue := rply.Payload[0]
	var val uint64
	var ok bool
	if val, ok = returnValue.(uint64); !ok {
		return val, fmt.Errorf("_IDE_initiateControlSessionWithProtocolVersion got wrong returnvalue: %s", rply.Payload)
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("initiateControlSessionForTestProcessID reply")
	return val, nil
}

func (xdc XCTestManager_DaemonConnectionInterface) initiateControlSession(pid uint64, version uint64) error {
	_, err := xdc.IDEDaemonProxy.MethodCall("_IDE_initiateControlSessionForTestProcessID:protocolVersion:", pid, version)
	return err
}

func startExecutingTestPlanWithProtocolVersion(channel *dtx.Channel, protocolVersion uint64) error {
	rply, err := channel.MethodCall("_IDE_startExecutingTestPlanWithProtocolVersion:", protocolVersion)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{"channel_id": ideToDaemonProxyChannelName, "reply": rply}).Debug("_IDE_startExecutingTestPlanWithProtocolVersion reply")
	return nil
}

const ideToDaemonProxyChannelName = "dtxproxy:XCTestManager_IDEInterface:XCTestManager_DaemonConnectionInterface"

type dtxproxy struct {
	ideInterface     XCTestManager_IDEInterface
	daemonConnection XCTestManager_DaemonConnectionInterface
	IDEDaemonProxy   *dtx.Channel
	dtxConnection    *dtx.Connection
	proxyDispatcher  proxyDispatcher
}

func newDtxProxy(dtxConnection *dtx.Connection) dtxproxy {
	testBundleReadyChannel := make(chan dtx.Message, 1)
	//(xide XCTestManager_IDEInterface)
	proxyDispatcher := proxyDispatcher{testBundleReadyChannel: testBundleReadyChannel, dtxConnection: dtxConnection}
	IDEDaemonProxy := dtxConnection.RequestChannelIdentifier(ideToDaemonProxyChannelName, proxyDispatcher)
	ideInterface := XCTestManager_IDEInterface{IDEDaemonProxy: IDEDaemonProxy, testBundleReadyChannel: testBundleReadyChannel}

	return dtxproxy{
		IDEDaemonProxy:   IDEDaemonProxy,
		ideInterface:     ideInterface,
		daemonConnection: XCTestManager_DaemonConnectionInterface{IDEDaemonProxy},
		dtxConnection:    dtxConnection,
		proxyDispatcher:  proxyDispatcher,
	}
}

func newDtxProxyWithConfig(dtxConnection *dtx.Connection, testConfig nskeyedarchiver.XCTestConfiguration, testListener *TestListener) dtxproxy {
	testBundleReadyChannel := make(chan dtx.Message, 1)
	//(xide XCTestManager_IDEInterface)
	proxyDispatcher := proxyDispatcher{
		testBundleReadyChannel:          testBundleReadyChannel,
		dtxConnection:                   dtxConnection,
		testRunnerReadyWithCapabilities: testRunnerReadyWithCapabilitiesConfig(testConfig),
		testListener:                    testListener,
	}
	IDEDaemonProxy := dtxConnection.RequestChannelIdentifier(ideToDaemonProxyChannelName, proxyDispatcher)
	ideInterface := XCTestManager_IDEInterface{IDEDaemonProxy: IDEDaemonProxy, testConfig: testConfig, testBundleReadyChannel: testBundleReadyChannel}

	return dtxproxy{
		IDEDaemonProxy:   IDEDaemonProxy,
		ideInterface:     ideInterface,
		daemonConnection: XCTestManager_DaemonConnectionInterface{IDEDaemonProxy},
		dtxConnection:    dtxConnection,
		proxyDispatcher:  proxyDispatcher,
	}
}

const (
	testmanagerd      = "com.apple.testmanagerd.lockdown"
	testmanagerdiOS14 = "com.apple.testmanagerd.lockdown.secure"
	testmanagerdiOS17 = "com.apple.dt.testmanagerd.remote"
)

const testBundleSuffix = "UITests.xctrunner"

// TestConfig specifies the parameters of a test execution
type TestConfig struct {
	// The identifier of the app under test
	BundleId string
	// The identifier of the test runner. For unit tests (non-UI tests) this is also the
	// app under test (BundleId can be left empty) as the .xctest bundle is packaged into the app under test
	TestRunnerBundleId string
	// XctestConfigName is the name of the
	XctestConfigName string
	// Env is passed as environment variables to the test runner
	Env map[string]any
	// Args are passed to the test runner as launch arguments
	Args []string
	// TestsToRun specifies a list of tests that should be executed. All other tests are ignored. To execute all tests
	// pass nil.
	// The format of the values is {PRODUCT_MODULE_NAME}.{CLASS}/{METHOD} where {PRODUCT_MODULE_NAME} and {METHOD} are
	// optional. If {METHOD} is omitted, all tests of {CLASS} are executed
	TestsToRun []string
	// TestsToSkip specifies a list of tests that should be skipped. See TestsToRun for the format
	TestsToSkip []string
	// XcTest needs to be set to true if the TestRunnerBundleId is a unit test and not a UI test
	XcTest bool
	// The device on which the test is executed
	Device ios.DeviceEntry
	// The listener for receiving results
	Listener *TestListener
}

func StartXCTestWithConfig(ctx context.Context, xctestrunFilePath string, device ios.DeviceEntry, listener *TestListener) ([]TestSuite, error) {
	xctestConfigurations, err := parseFile(xctestrunFilePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing xctestrun file: %w", err)
	}
	installedApps := getUserInstalledApps(err, device)
	var xcTestTargets []TestConfig
	for _, xctestSpecification := range xctestConfigurations {
		for i, r := range xctestSpecification.TestTargets {
			tc, err := r.buildTestConfig(device, listener, installedApps)
			if err != nil {
				return nil, fmt.Errorf("building test config at index %d: %w", i, err)
			}
			xcTestTargets = append(xcTestTargets, tc)
		}
	}

	var results []TestSuite
	var targetErrors []error
	for _, target := range xcTestTargets {
		listener.reset()
		suites, err := RunTestWithConfig(ctx, target)
		if err != nil {
			targetErrors = append(targetErrors, err)
		}
		results = append(results, suites...)
	}

	return results, errors.Join(targetErrors...)
}

func RunTestWithConfig(ctx context.Context, testConfig TestConfig) ([]TestSuite, error) {
	if len(testConfig.TestRunnerBundleId) == 0 {
		return nil, fmt.Errorf("RunTestWithConfig: testConfig.TestRunnerBundleId can not be empty")
	}
	version, err := ios.GetProductVersion(testConfig.Device)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("RunXCUIWithBundleIdsCtx: cannot determine iOS version: %w", err)
	}

	if version.LessThan(ios.IOS14()) {
		log.Debugf("iOS version: %s detected, running with ios11 support", version)
		return runXCUIWithBundleIdsXcode11Ctx(ctx, testConfig, version)
	}

	if version.LessThan(ios.IOS17()) {
		log.Debugf("iOS version: %s detected, running with ios14 support", version)
		return runXUITestWithBundleIdsXcode12Ctx(ctx, testConfig, version)
	}

	log.Debugf("iOS version: %s detected, running with ios17 support", version)
	return runXUITestWithBundleIdsXcode15Ctx(ctx, testConfig, version)
}

func runXUITestWithBundleIdsXcode15Ctx(
	ctx context.Context,
	config TestConfig,
	version *semver.Version,
) ([]TestSuite, error) {
	conn1, err := dtx.NewTunnelConnection(config.Device, testmanagerdiOS17)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot create a tunnel connection to testmanagerd: %w", err)
	}
	defer conn1.Close()

	conn2, err := dtx.NewTunnelConnection(config.Device, testmanagerdiOS17)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot create a tunnel connection to testmanagerd: %w", err)
	}
	defer conn2.Close()

	installationProxy, err := installationproxy.New(config.Device)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot connect to installation proxy: %w", err)
	}
	defer installationProxy.Close()
	apps, err := installationProxy.BrowseUserApps()
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot browse user apps: %w", err)
	}

	testAppInfo, err := getappInfo(config.TestRunnerBundleId, apps)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot get test app information: %w", err)
	}

	info := testInfo{
		testApp: testAppInfo,
	}

	if config.BundleId != "" {
		appInfo, err := getappInfo(config.BundleId, apps)
		if err != nil {
			return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot get app information: %w", err)
		}

		info.targetApp = appInfo
	}

	testSessionID := uuid.New()
	testconfig := createTestConfig(info, testSessionID, config.XctestConfigName, config.TestsToRun, config.TestsToSkip, config.XcTest, version)
	ideDaemonProxy1 := newDtxProxyWithConfig(conn1, testconfig, config.Listener)

	localCaps := nskeyedarchiver.XCTCapabilities{CapabilitiesDictionary: map[string]interface{}{
		"XCTIssue capability":                      uint64(1),
		"daemon container sandbox extension":       uint64(1),
		"delayed attachment transfer":              uint64(1),
		"expected failure test capability":         uint64(1),
		"request diagnostics for specific devices": uint64(1),
		"skipped test capability":                  uint64(1),
		"test case run configurations":             uint64(1),
		"test iterations":                          uint64(1),
		"test timeout capability":                  uint64(1),
		"ubiquitous test identifiers":              uint64(1),
	}}
	receivedCaps, err := ideDaemonProxy1.daemonConnection.initiateSessionWithIdentifierAndCaps(testSessionID, localCaps)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot initiate a IDE session: %w", err)
	}
	log.WithField("receivedCaps", receivedCaps).Info("got capabilities")

	appserviceConn, err := appservice.New(config.Device)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot connect to app service: %w", err)
	}
	defer appserviceConn.Close()

	testRunnerLaunch, err := startTestRunner17(appserviceConn, config.TestRunnerBundleId, strings.ToUpper(testSessionID.String()), info.testApp.path+"/PlugIns/"+config.XctestConfigName, config.Args, config.Env, config.XcTest)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot start test runner: %w", err)
	}

	defer testRunnerLaunch.Close()
	go func() {
		_, err := io.Copy(config.Listener.logWriter, testRunnerLaunch)
		if err != nil {
			log.Warn("copying stdout failed", log.WithError(err))
		}
	}()

	ideDaemonProxy2 := newDtxProxyWithConfig(conn2, testconfig, config.Listener)
	caps, err := ideDaemonProxy2.daemonConnection.initiateControlSessionWithCapabilities(nskeyedarchiver.XCTCapabilities{CapabilitiesDictionary: map[string]interface{}{}})
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot initiate a control session with capabilities: %w", err)
	}
	log.WithField("caps", caps).Info("got capabilities")
	authorized, err := ideDaemonProxy2.daemonConnection.authorizeTestSessionWithProcessID(uint64(testRunnerLaunch.Pid))
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot authorize test session: %w", err)
	}
	log.WithField("authorized", authorized).Info("authorized")

	ideInterfaceChannel := ideDaemonProxy1.dtxConnection.ForChannelRequest(proxyDispatcher{id: "dtxproxy:XCTestDriverInterface:XCTestManager_IDEInterface"})

	proto := uint64(36)
	err = ideDaemonProxy1.daemonConnection.startExecutingTestPlanWithProtocolVersion(ideInterfaceChannel, proto)
	if err != nil {
		return make([]TestSuite, 0), fmt.Errorf("runXUITestWithBundleIdsXcode15Ctx: cannot start executing test plan: %w", err)
	}

	select {
	case <-conn1.Closed():
		log.Debug("conn1 closed")
		if !errors.Is(conn1.Err(), dtx.ErrConnectionClosed) {
			log.WithError(conn1.Err()).Error("conn1 closed unexpectedly")
		}
		config.Listener.FinishWithError(errors.New("lost connection to testmanagerd. the test-runner may have been killed"))
		break
	case <-conn2.Closed():
		log.Debug("conn2 closed")
		if !errors.Is(conn2.Err(), dtx.ErrConnectionClosed) {
			log.WithError(conn2.Err()).Error("conn2 closed unexpectedly")
		}
		config.Listener.FinishWithError(errors.New("lost connection to testmanagerd. the test-runner may have been killed"))
		break
	case <-config.Listener.Done():
		break
	case <-ctx.Done():
		break
	}
	log.Infof("Killing test runner with pid %d ...", testRunnerLaunch.Pid)
	err = killTestRunner(appserviceConn, testRunnerLaunch.Pid)
	if err != nil {
		log.Infof("Nothing to kill, process with pid %d is already dead", testRunnerLaunch.Pid)
	} else {
		log.Info("Test runner killed with success")
	}

	log.Debugf("Done running test")

	return config.Listener.TestSuites, config.Listener.err
}

type processKiller interface {
	KillProcess(pid int) error
}

func killTestRunner(killer processKiller, pid int) error {
	log.Infof("Killing test runner with pid %d ...", pid)
	err := killer.KillProcess(pid)
	if err != nil {
		return err
	}
	log.Info("Test runner killed with success")

	return nil
}

func startTestRunner17(appserviceConn *appservice.Connection, bundleID string, sessionIdentifier string, testBundlePath string, testArgs []string, testEnv map[string]interface{}, isXCTest bool) (appservice.LaunchedAppWithStdIo, error) {
	args := []interface{}{}
	for _, arg := range testArgs {
		args = append(args, arg)
	}

	libraries := "/Developer/usr/lib/libMainThreadChecker.dylib"
	if isXCTest {
		libraries += ":/System/Developer/usr/lib/libXCTestBundleInject.dylib"
	}

	env := map[string]interface{}{
		"CA_ASSERT_MAIN_THREAD_TRANSACTIONS": "0",
		"CA_DEBUG_TRANSACTIONS":              "0",
		"DYLD_INSERT_LIBRARIES":              libraries,
		"DYLD_FRAMEWORK_PATH":                "/System/Developer/Library/Frameworks",
		"DYLD_LIBRARY_PATH":                  "/System/Developer/usr/lib",

		"MTC_CRASH_ON_REPORT":             "1",
		"NSUnbufferedIO":                  "YES",
		"OS_ACTIVITY_DT_MODE":             "YES",
		"SQLITE_ENABLE_THREAD_ASSERTIONS": "1",
		"XCTestBundlePath":                testBundlePath,
		"XCTestConfigurationFilePath":     "",
		"XCTestManagerVariant":            "DDI",
		"XCTestSessionIdentifier":         strings.ToUpper(sessionIdentifier),
	}

	if len(testEnv) > 0 {
		maps.Copy(env, testEnv)

		for key, value := range testEnv {
			log.Debugf("adding extra env %s=%s", key, value)
		}
	}
	var opts = map[string]interface{}{}

	if !isXCTest {
		opts = map[string]interface{}{
			"ActivateSuspended":   uint64(1),
			"StartSuspendedKey":   uint64(0),
			"__ActivateSuspended": uint64(1),
		}
	}

	appLaunch, err := appserviceConn.LaunchAppWithStdIo(
		bundleID,
		args,
		env,
		opts,
		true,
	)

	if err != nil {
		return appservice.LaunchedAppWithStdIo{}, err
	}

	return appLaunch, nil
}

func setupXcuiTest(device ios.DeviceEntry, bundleID string, testRunnerBundleID string, xctestConfigFileName string, testsToRun []string, testsToSkip []string, isXCTest bool, version *semver.Version) (uuid.UUID, string, nskeyedarchiver.XCTestConfiguration, testInfo, error) {
	testSessionID := uuid.New()
	installationProxy, err := installationproxy.New(device)
	if err != nil {
		return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
	}
	defer installationProxy.Close()

	apps, err := installationProxy.BrowseUserApps()
	if err != nil {
		return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
	}

	testAppInfo, err := getappInfo(testRunnerBundleID, apps)
	if err != nil {
		return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
	}

	info := testInfo{
		testApp: testAppInfo,
	}

	if bundleID != "" {
		appInfo, err := getappInfo(bundleID, apps)
		if err != nil {
			return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
		}
		log.Debugf("app info found: %+v", appInfo)

		info.targetApp = appInfo
	}

	houseArrestService, err := house_arrest.New(device, testRunnerBundleID)
	defer houseArrestService.Close()
	if err != nil {
		return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
	}
	log.Debugf("creating test config")
	testConfigPath, testConfig, err := createTestConfigOnDevice(testSessionID, info, houseArrestService, xctestConfigFileName, testsToRun, testsToSkip, isXCTest, version)
	if err != nil {
		return uuid.UUID{}, "", nskeyedarchiver.XCTestConfiguration{}, testInfo{}, err
	}

	return testSessionID, testConfigPath, testConfig, info, nil
}

func createTestConfigOnDevice(testSessionID uuid.UUID, info testInfo, houseArrestService *house_arrest.Connection, xctestConfigFileName string, testsToRun []string, testsToSkip []string, isXCTest bool, version *semver.Version) (string, nskeyedarchiver.XCTestConfiguration, error) {
	relativeXcTestConfigPath := path.Join("tmp", testSessionID.String()+".xctestconfiguration")
	xctestConfigPath := path.Join(info.testApp.homePath, relativeXcTestConfigPath)

	testBundleURL := path.Join(info.testApp.path, "PlugIns", xctestConfigFileName)

	productModuleName := strings.ReplaceAll(xctestConfigFileName, ".xctest", "")
	config := nskeyedarchiver.NewXCTestConfiguration(productModuleName, testSessionID, info.targetApp.bundleID, info.targetApp.path, testBundleURL, testsToRun, testsToSkip, isXCTest, version)
	result, err := nskeyedarchiver.ArchiveXML(config)
	if err != nil {
		return "", nskeyedarchiver.XCTestConfiguration{}, err
	}

	err = houseArrestService.SendFile([]byte(result), relativeXcTestConfigPath)
	if err != nil {
		return "", nskeyedarchiver.XCTestConfiguration{}, err
	}
	return xctestConfigPath, nskeyedarchiver.NewXCTestConfiguration(productModuleName, testSessionID, info.targetApp.bundleID, info.targetApp.path, testBundleURL, testsToRun, testsToSkip, isXCTest, version), nil
}

func createTestConfig(info testInfo, testSessionID uuid.UUID, xctestConfigFileName string, testsToRun []string, testsToSkip []string, isXCTest bool, version *semver.Version) nskeyedarchiver.XCTestConfiguration {
	// the default value for this generated by Xcode is the target name, and the same name is used for the '.xctest' bundle name per default
	productModuleName := strings.ReplaceAll(xctestConfigFileName, ".xctest", "")
	return nskeyedarchiver.NewXCTestConfiguration(productModuleName, testSessionID, info.targetApp.bundleID, info.targetApp.path, "PlugIns/"+xctestConfigFileName, testsToRun, testsToSkip, isXCTest, version)
}

type testInfo struct {
	testApp   appInfo
	targetApp appInfo // Optional
}

type appInfo struct {
	path       string
	bundleName string
	bundleID   string
	homePath   string
}

func getappInfo(bundleID string, apps []installationproxy.AppInfo) (appInfo, error) {
	for _, app := range apps {
		if app.CFBundleIdentifier() == bundleID {
			info := appInfo{
				path:       app.Path(),
				bundleName: app.CFBundleName(),
				bundleID:   app.CFBundleIdentifier(),
			}
			if home, ok := app.EnvironmentVariables()["HOME"].(string); ok {
				info.homePath = home
			}
			return info, nil
		}
	}

	return appInfo{}, fmt.Errorf("Did not find test app for '%s' on device. Is it installed?", bundleID)
}

func getUserInstalledApps(err error, device ios.DeviceEntry) []installationproxy.AppInfo {
	svc, err := installationproxy.New(device)
	if err != nil {
		log.WithError(err).Debug("we couldn't create ios device connection")
		return nil
	}
	defer svc.Close()
	installedApps, err := svc.BrowseUserApps()
	if err != nil {
		log.WithError(err).Debug("we couldn't fetch the installed user apps")
		return nil
	}
	return installedApps
}
