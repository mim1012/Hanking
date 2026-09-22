// ntsys_amd64.s — minimal indirect-syscall trampoline.
//
// Executes an ntdll syscall by number instead of calling through the (possibly
// EDR-hooked) ntdll export. Arguments follow the Windows x64 calling convention:
//   param1 → rcx (then moved to r10), param2-4 → rdx/r8/r9, param5-6 → stack.
//
//go:build windows && amd64

#include "textflag.h"

// func Syscall(num uintptr, a1, a2, a3, a4, a5, a6 uintptr) uintptr
TEXT ·Syscall(SB), NOSPLIT, $0-64
	MOVQ num+0(FP), AX   // syscall number
	MOVQ a1+8(FP), CX    // param1
	MOVQ a2+16(FP), DX   // param2
	MOVQ a3+24(FP), R8   // param3
	MOVQ a4+32(FP), R9   // param4
	MOVQ a5+40(FP), R10  // param5
	MOVQ a6+48(FP), R11  // param6
	MOVQ R10, 0x28(SP)   // [rsp+0x28] = param5
	MOVQ R11, 0x30(SP)   // [rsp+0x30] = param6
	MOVQ CX, R10         // ntdll expects param1 in r10
	SYSCALL
	MOVQ AX, ret+56(FP)  // return NTSTATUS
	RET
