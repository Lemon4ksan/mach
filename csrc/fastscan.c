// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <immintrin.h>
#include <stdint.h>
#include <stddef.h>

// scan_byte_avx2 scans data for target byte using 256-bit AVX2 SIMD.
// Returns 0-based index of first match, or -1 if not found.
int64_t scan_byte_avx2(const uint8_t* data, uint64_t len, uint8_t target) {
    __m128i target_128 = _mm_cvtsi32_si128(target);
    __m256i target_vec = _mm256_broadcastb_epi8(target_128);
    size_t i = 0;

    for (; i + 32 <= len; i += 32) {
        __m256i chunk = _mm256_loadu_si256((const __m256i*)(data + i));
        __m256i cmp = _mm256_cmpeq_epi8(chunk, target_vec);
        uint32_t mask = (uint32_t)_mm256_movemask_epi8(cmp);
        if (mask != 0) {
            return (int64_t)(i + __builtin_ctz(mask));
        }
    }

    for (; i < len; i++) {
        if (data[i] == target) return (int64_t)i;
    }

    return -1;
}

// scan_crlfcrlf_avx2 searches for "\r\n\r\n" in data.
// Returns the index of the first byte after "\r\n\r\n", or -1 if not found.
int64_t scan_crlfcrlf_avx2(const uint8_t* data, uint64_t len) {
    if (len < 4) return -1;
    
    // Use volatile or runtime register to ensure no .rodata memory reference
    volatile uint8_t cr = 0x0D;
    __m128i r_128 = _mm_cvtsi32_si128(cr);
    __m256i r_vec = _mm256_broadcastb_epi8(r_128);
    size_t i = 0;

    for (; i + 32 <= len; i += 32) {
        __m256i chunk = _mm256_loadu_si256((const __m256i*)(data + i));
        __m256i cmp = _mm256_cmpeq_epi8(chunk, r_vec);
        uint32_t mask = (uint32_t)_mm256_movemask_epi8(cmp);

        while (mask != 0) {
            uint32_t bit = (uint32_t)__builtin_ctz(mask);
            size_t pos = i + bit;

            if (pos + 3 < len) {
                uint32_t val = *(const uint32_t*)(data + pos);
                if (val == 0x0A0D0A0D) {
                    return (int64_t)(pos + 4);
                }
            }

            mask &= mask - 1;
        }
    }

    for (; i < len; i++) {
        if (data[i] == '\r' && i + 3 < len) {
            uint32_t val = *(const uint32_t*)(data + i);
            if (val == 0x0A0D0A0D) {
                return (int64_t)(i + 4);
            }
        }
    }

    return -1;
}
