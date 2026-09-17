# Журнал экспериментов (Tracker)

Здесь фиксируются результаты микро-бенчмарков для подтверждения профита.

| ID | Гипотеза / Фаза | Цель RFC | Status | До (МБ/с | ns/op) | После (МБ/с | ns/op) | Аллокации (До -> После) | Вывод |
| H-00 | Baseline QPACK Decode | 0 allocs | Baseline | - | 752.5 ns/op | - | - | 8 allocs | - |
| H-00 | Baseline Varint Parse (8b) | 0 allocs | Baseline | - | 3.656 ns/op | - | - | 0 allocs | - |
| H-01 | Phase 2: QPACK Arena | 0 allocs | Success | 752.5 | 247.5 ns/op | - | - | 8 -> 0 allocs | 3x speedup, 8 allocs saved (Absolute Zero) |
| H-02 | Phase 5: In-Situ Overlay | 0 allocs | Success | 325 | 5.8 ns/op | - | - | 2 -> 0 allocs | 55x speedup per frame! |
| H-03 | Phase 4: Lock-free Matrix | 0 allocs | Success | 65.73 | 1.47 ns/op | - | - | 0 -> 0 allocs | 44x speedup per packet lookup |
