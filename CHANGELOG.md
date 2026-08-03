# Changelog

## v2.2.0 — Visual UI Configurator

- добавлен отдельный экран настройки физической клавиатуры 22+4;
- редактируются EN/RU и восемь semantic-режимов каждой основной клавиши;
- добавлены действия `text`, `key`, `combo`, `command`, `macro` и отключение клавиши;
- добавлена настройка коротких действий THUMB_1, THUMB_2 и THUMB_4;
- роли удержания больших клавиш отображаются и связываются с соответствующими режимами;
- добавлены профили, дублирование, переименование, активация и удаление;
- изменения применяются через atomic resolver без перезапуска;
- добавлены сохранение, откат, строгий импорт и экспорт JSON;
- добавлены показатели пустых назначений, дублей и фоновых действий;
- добавлен проверочный запуск выбранного действия;
- удалён старый read-only диалог четырёх legacy-слоёв;
- добавлены тесты конфигурации, профилей и модели редактора.


## 2.1.0-lowlatency — 2026-08-03

- Reduced press debounce from 8 ms to 2.5 ms and release debounce to 4.5 ms.
- Increased firmware scan rate to approximately 4 kHz.
- Added RP2040-wide GPIO snapshot scanning.
- Prioritized physical input over host command processing.
- Added firmware latency and host-backlog regression tests.
- Split realtime input, macro steps and background commands into independent queues.
- Made realtime input lossless under queue pressure.
- Batched Windows `SendInput` down/up events.
- Added a dedicated Above Normal priority Windows input thread.
- Removed per-keystroke logging and critical-path GUI work.
- Added zero-allocation semantic stroke lookup and single-rune Unicode injection.
- Reduced macro step delay from 50 ms to 12 ms.

## 2.0.0 — 2026-08-02

- Introduced 22+4 dual-role text input architecture and protocol v2.
- Added deterministic English/Russian Unicode companion input.
