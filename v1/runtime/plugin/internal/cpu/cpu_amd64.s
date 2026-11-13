//go:build amd64 && !purego

#include "textflag.h"

// func _Cpuid(leaf uint32, sub uint32) (eax uint32, ebx uint32, ecx uint32, edx uint32)
// Same shape as internal/cpu/cpu_x86.s.
TEXT ·_Cpuid(SB), NOSPLIT, $0-24
	MOVL leaf+0(FP), AX
	MOVL sub+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

// func _Xgetbv() (eax uint32, edx uint32)
TEXT ·_Xgetbv(SB), NOSPLIT, $0-8
	MOVL $0, CX
	XGETBV
	MOVL AX, eax+0(FP)
	MOVL DX, edx+4(FP)
	RET
