#pragma once

#include <Arduino.h>
#include <Keyboard.h>
#include <Mouse.h>

#include "device_config.h"

namespace AppConfig {

constexpr uint8_t MAX_ACTION_KEYS = 4;
constexpr uint8_t MAX_CHORD_COMBOS = 14;
constexpr uint8_t NO_BUTTON = 0xFF;

// ── Action types ────────────────────────────────────────────────────────────

enum class ActionKind : uint8_t {
    None, Keyboard, Consumer, MouseButton, MouseScroll,
};

struct ButtonAction {
    ActionKind kind;
    uint8_t keyCount;
    uint8_t keys[MAX_ACTION_KEYS];
    uint16_t consumerKey;
};

constexpr ButtonAction noAction() { return {ActionKind::None, 0, {0,0,0,0}, 0}; }
constexpr ButtonAction keyboardKey(uint8_t k) { return {ActionKind::Keyboard, 1, {k,0,0,0}, 0}; }
constexpr ButtonAction keyboardCombo(uint8_t a, uint8_t b) { return {ActionKind::Keyboard, 2, {a,b,0,0}, 0}; }
constexpr ButtonAction consumerKey(uint16_t k) { return {ActionKind::Consumer, 0, {0,0,0,0}, k}; }
constexpr ButtonAction mouseButton(uint8_t b) { return {ActionKind::MouseButton, 1, {b,0,0,0}, 0}; }
constexpr ButtonAction mouseScrollUp() { return {ActionKind::MouseScroll, 0, {1,0,0,0}, 0}; }
constexpr ButtonAction mouseScrollDown() { return {ActionKind::MouseScroll, 0, {0xFF,0,0,0}, 0}; }

// ── Chord combo types ───────────────────────────────────────────────────────

struct ChordBinding {
    uint8_t buttonA;
    uint8_t buttonB;
    ButtonAction action;
};

struct ChordConfig {
    uint8_t count;
    ChordBinding chords[MAX_CHORD_COMBOS];
};

// ── Russian HID keycodes (ЙЦУКЕН on QWERTY) ────────────────────────────────

enum class Ru : uint8_t {
    A,Be,Ve,Ge,De,E,Yo,Zhe,Ze,I,ShortI,Ka,El,Em,En,O,Pe,Er,Es,Te,U,
    Ef,Ha,Tse,Che,Sha,Shcha,Hard,Yeri,Soft,Eh,Yu,Ya,
};

constexpr uint8_t rk(Ru l) {
    switch(l) {
        case Ru::A:return'f'; case Ru::Be:return','; case Ru::Ve:return'd';
        case Ru::Ge:return'u'; case Ru::De:return'l'; case Ru::E:return't';
        case Ru::Yo:return'`'; case Ru::Zhe:return';'; case Ru::Ze:return'p';
        case Ru::I:return'b'; case Ru::ShortI:return'q'; case Ru::Ka:return'r';
        case Ru::El:return'k'; case Ru::Em:return'v'; case Ru::En:return'y';
        case Ru::O:return'j'; case Ru::Pe:return'g'; case Ru::Er:return'h';
        case Ru::Es:return'c'; case Ru::Te:return'n'; case Ru::U:return'e';
        case Ru::Ef:return'a'; case Ru::Ha:return'['; case Ru::Tse:return'w';
        case Ru::Che:return'x'; case Ru::Sha:return'i'; case Ru::Shcha:return'o';
        case Ru::Hard:return']'; case Ru::Yeri:return's'; case Ru::Soft:return'm';
        case Ru::Eh:return'\''; case Ru::Yu:return'.'; case Ru::Ya:return'z';
        default: return 0;
    }
}
constexpr ButtonAction ru(Ru l) { return keyboardKey(rk(l)); }

// ── Button index reference ──────────────────────────────────────────────────
//  0=I_1  1=I_2  2=I_3  3=I_4  4=I_5  5=I_6
//  6=M_1  7=M_2  8=M_3  9=M_4  10=M_5
//  11=R_1 12=R_2 13=R_3 14=R_4 15=R_5
//  16=P_1 17=P_2 18=P_3 19=P_4 20=P_5 21=P_6
//
// Priority: 3(Push)>4(Far)>2(Base)>1(Pull)>5(Ext)>6(Aux)
// Fingers:  Middle > Index > Ring > Pinky
// I_2, M_2 = Mouse L/R on all layers

// ── KEYMAP ──────────────────────────────────────────────────────────────────

constexpr ButtonAction KEYMAP[LAYER_COUNT][MAIN_BUTTONS] = {
    { // Layer 0: Digits + Symbols + Mouse
      // I_1..I_6
      keyboardKey('1'), mouseButton(MOUSE_LEFT), keyboardKey('2'), keyboardKey('3'),
      keyboardKey('4'), keyboardKey('5'),
      // M_1..M_5
      keyboardKey('6'), mouseButton(MOUSE_RIGHT), keyboardKey('7'), keyboardKey('8'),
      keyboardKey('9'),
      // R_1..R_5
      keyboardKey('0'), keyboardKey('='), keyboardKey('-'), keyboardKey('['),
      keyboardKey(']'),
      // P_1..P_6
      keyboardKey('\\'), keyboardKey('/'), keyboardKey('.'), keyboardKey(';'),
      keyboardKey('\''), keyboardKey('`'),
    },
    { // Layer 1: English (frequency-optimised, I_2/M_2 = mouse)
      // Priority: M3=E, I3=T, R3=A, P3=O, M4=I, I4=N, R4=S, P4=H
      //           R2=R, P2=D, M1=L, I1=C, R1=U, P1=M
      //           M5=W, I5=F, R5=G, P5=Y, I6=P, P6=B
      keyboardKey('c'), mouseButton(MOUSE_LEFT), keyboardKey('t'), keyboardKey('n'),
      keyboardKey('f'), keyboardKey('p'),
      keyboardKey('l'), mouseButton(MOUSE_RIGHT), keyboardKey('e'), keyboardKey('i'),
      keyboardKey('w'),
      keyboardKey('u'), keyboardKey('r'), keyboardKey('a'), keyboardKey('s'),
      keyboardKey('g'),
      keyboardKey('m'), keyboardKey('d'), keyboardKey('o'), keyboardKey('h'),
      keyboardKey('y'), keyboardKey('b'),
    },
    { // Layer 2: Russian (frequency-optimised, I_2/M_2 = mouse)
      // M3=О, I3=Е, R3=А, P3=И, M4=Н, I4=Т, R4=С, P4=Р
      // R2=В, P2=Л, M1=К, I1=М, R1=Д, P1=П
      // M5=У, I5=Я, R5=Ы, P5=Ь, I6=Г, P6=З
      ru(Ru::Em), mouseButton(MOUSE_LEFT), ru(Ru::E), ru(Ru::Te),
      ru(Ru::Ya), ru(Ru::Ge),
      ru(Ru::Ka), mouseButton(MOUSE_RIGHT), ru(Ru::O), ru(Ru::En),
      ru(Ru::U),
      ru(Ru::De), ru(Ru::Ve), ru(Ru::A), ru(Ru::Es),
      ru(Ru::Yeri),
      ru(Ru::Pe), ru(Ru::El), ru(Ru::I), ru(Ru::Er),
      ru(Ru::Soft), ru(Ru::Ze),
    },
    { // Layer 3: Service (I_2/M_2 = mouse)
      keyboardKey(KEY_PAGE_DOWN), mouseButton(MOUSE_LEFT),
      keyboardKey(KEY_LEFT_SHIFT), keyboardKey(KEY_LEFT_GUI),
      keyboardKey(KEY_ESC), keyboardKey(KEY_TAB),
      keyboardKey(KEY_PAGE_UP), mouseButton(MOUSE_RIGHT),
      keyboardKey(KEY_LEFT_CTRL), keyboardKey(KEY_LEFT_ALT),
      keyboardKey(KEY_HOME),
      keyboardKey(KEY_INSERT), keyboardKey(KEY_END),
      keyboardKey(KEY_DELETE), consumerKey(KEY_PLAY_PAUSE),
      consumerKey(KEY_MUTE),
      keyboardKey(KEY_LEFT_ARROW), keyboardKey(KEY_RIGHT_ARROW),
      keyboardKey(KEY_UP_ARROW), keyboardKey(KEY_DOWN_ARROW),
      consumerKey(KEY_VOLUME_INCREMENT), consumerKey(KEY_VOLUME_DECREMENT),
    },
};

// ── Thumb button actions (layer-independent) ────────────────────────────────

constexpr ButtonAction THUMB_ACTIONS[THUMB_BUTTONS] = {
    keyboardKey(' '),                         // THUMB_1 = Space
    keyboardKey(KEY_RETURN),                  // THUMB_2 = Enter
    keyboardCombo(KEY_LEFT_GUI, ' '),         // THUMB_3 = Win+Space (Lang)
    keyboardKey(KEY_BACKSPACE),               // THUMB_4 = Backspace
};

// ── Chord combos per layer ──────────────────────────────────────────────────
// Two-key chords: press both within COMBO_TERM_MS → rare letter.
// Second button pressed triggers the chord; first button's output is undone.

constexpr ChordConfig CHORD_COMBOS[LAYER_COUNT] = {
    // Layer 0: no chords
    {0, {}},
    // Layer 1: English rare letters
    {6, {
        {4,  10, keyboardKey('v')},  // I5(F)+M5(W)  -> V
        {0,   6, keyboardKey('k')},  // I1(C)+M1(L)  -> K
        {19,  3, keyboardKey('j')},  // P4(H)+I4(N)  -> J
        {0,  14, keyboardKey('x')},  // I1(C)+R4(S)  -> X
        {6,  14, keyboardKey('q')},  // M1(L)+R4(S)  -> Q
        {14, 13, keyboardKey('z')},  // R4(S)+R3(A)  -> Z
    }},
    // Layer 2: Russian rare letters
    {12, {
        {12, 17, ru(Ru::Be)},       // R2(В)+P2(Л)  -> Б
        {9,   3, ru(Ru::Che)},      // M4(Н)+I4(Т)  -> Ч
        {3,   2, ru(Ru::ShortI)},   // I4(Т)+I3(Е)  -> Й
        {19, 14, ru(Ru::Zhe)},      // P4(Р)+R4(С)  -> Ж
        {14,  3, ru(Ru::Sha)},      // R4(С)+I4(Т)  -> Ш
        {10,  4, ru(Ru::Yu)},       // M5(У)+I5(Я)  -> Ю
        {14, 13, ru(Ru::Tse)},      // R4(С)+R3(А)  -> Ц
        {14, 20, ru(Ru::Shcha)},    // R4(С)+P5(Ь)  -> Щ
        {2,  13, ru(Ru::Eh)},       // I3(Е)+R3(А)  -> Э
        {16, 11, ru(Ru::Ef)},       // P1(П)+R1(Д)  -> Ф
        {20, 21, ru(Ru::Hard)},     // P5(Ь)+P6(З)  -> Ъ
        {2,   8, ru(Ru::Yo)},       // I3(Е)+M3(О)  -> Ё
    }},
    // Layer 3: no chords
    {0, {}},
};

}  // namespace AppConfig
