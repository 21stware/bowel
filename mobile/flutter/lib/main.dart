import 'dart:io' show Platform;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart' show KeyDownEvent, LogicalKeyboardKey, SystemNavigator;

import 'app/sample.dart';
import 'app/shell.dart';
import 'app/state.dart';
import 'board/board.dart';
import 'pages/activity.dart';
import 'pages/deco.dart';
import 'pages/frame.dart';
import 'pages/launch.dart';
import 'pages/providers.dart';
import 'pages/settings.dart';
import 'pages/sheet.dart';
import 'pages/setup.dart';
import 'ui/focus.dart';
import 'ui/skin.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  watchRemoteKeys();
  final app = AppState();
  Shell.instance.onTabSelected = (i) => app.selectTab(i, fromShell: true);
  await Shell.instance.start();
  await app.load();
  runApp(BowelApp(app: app, ready: app.attachCore()));
}

class BowelApp extends StatelessWidget {
  const BowelApp({super.key, required this.app, required this.ready});

  final AppState app;

  /// The core has started, while the launch animation plays.
  final Future<void> ready;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'bowel',
      theme: ThemeData(fontFamily: systemFamily, splashFactory: NoSplash.splashFactory, highlightColor: Colors.transparent),
      builder: (context, child) => _Canvas(child: child!),
      home: AnimatedBuilder(animation: app, builder: (context, _) => _Root(app: app, ready: ready)),
    );
  }
}

/// Draws a TV-shaped window at a desktop window's size, scaled down to the
/// screen ([canvasScaleOf]); anything else as it is. Over the navigator, so
/// sheets and dialogs scale with the pages.
class _Canvas extends StatelessWidget {
  const _Canvas({required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final mq = MediaQuery.of(context);
    final k = canvasScaleOf(mq.size);
    if (k == 1) return child;
    final size = mq.size / k;
    return FittedBox(
      fit: BoxFit.fill,
      alignment: Alignment.topLeft,
      child: SizedBox.fromSize(
        size: size,
        child: MediaQuery(
          data: mq.copyWith(
            size: size,
            devicePixelRatio: mq.devicePixelRatio * k,
            padding: mq.padding / k,
            viewPadding: mq.viewPadding / k,
            viewInsets: mq.viewInsets / k,
            systemGestureInsets: mq.systemGestureInsets / k,
          ),
          child: child,
        ),
      ),
    );
  }
}

class _Root extends StatefulWidget {
  const _Root({required this.app, required this.ready});
  final AppState app;
  final Future<void> ready;

  @override
  State<_Root> createState() => _RootState();
}

class _RootState extends State<_Root> {
  String? _styled;
  bool _open = false;

  void _syncShell(Look l) {
    final barHidden = widget.app.setup != 0 || !_open;
    final warn = providers.any((p) => p.warn != null);
    final key = '${l.id}|${l.isDark}|${l.accent.toARGB32()}|$barHidden|$warn';
    if (key == _styled) return;
    _styled = key;
    final shell = Shell.instance;
    shell.setStyle(l.accent, dark: l.darkGlass, paper: l.paper);
    shell.setBadge(2, on: warn, color: l.warn);
    shell.setBarHidden(barHidden);
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app;
    final l = Look.of(app.skin, app.wantsDark(MediaQuery.platformBrightnessOf(context)), custom: app.accent);
    _syncShell(l);
    return Skin(
      look: l,
      child: Material(
        type: MaterialType.transparency,
        child: MediaQuery.withClampedTextScaling(
          maxScaleFactor: 1,
          child: ColoredBox(
            color: l.paper,
            child: Launch(
              ready: widget.ready,
              onOpen: () => setState(() => _open = true),
              child: ValueListenableBuilder<EdgeInsets?>(
                valueListenable: Shell.instance.insets,
                builder: (context, native, _) {
                  final media = MediaQuery.paddingOf(context);
                  final frame = FrameData(
                    top: native?.top ?? media.top,
                    barInset: native?.bottom ?? 0,
                    nativeBar: Shell.instance.native,
                    homeIndicator: media.bottom,
                    form: windowFormOf(MediaQuery.sizeOf(context)),
                    // the pages keep off the bars at the top and bottom, not the sides
                    overscan: overscanOf(MediaQuery.sizeOf(context), EdgeInsets.only(top: native?.top ?? media.top, bottom: media.bottom)),
                  );
                  return Frame(
                    data: frame,
                    child: Stack(fit: StackFit.expand, children: [
                      app.setup != 0 ? _setup(app, frame) : _Tabs(app: app),
                      const IgnorePointer(child: Deco()),
                    ]),
                  );
                },
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// The first run, its two steps. Back on step 2 goes up to step 1, the link
/// added there cleared rather than added a second time; Back on step 1
/// leaves. A sheet over the page closes first. Each step is a level: the
/// D-pad starts on its first button, and back on step 1 on 继续.
Widget _setup(AppState app, FrameData frame) {
  final stepBack = app.setup == 2 && app.sheet == null;
  return PopScope(
    canPop: app.setup != 2,
    onPopInvokedWithResult: (didPop, _) {
      if (didPop || !stepBack) return;
      app.setupField.clear();
      app.set(() {
        app.setup = 1;
        app.setupLink = '';
        app.setupError = null;
      });
    },
    child: Padding(
      padding: frame.overscan,
      child: FocusLevel(take: false, entry: app.setup, initial: 'next', child: SetupPage(app: app)),
    ),
  );
}

class _Tabs extends StatefulWidget {
  const _Tabs({required this.app});
  final AppState app;

  @override
  State<_Tabs> createState() => _TabsState();
}

class _TabsState extends State<_Tabs> {
  /// The tabs in the bar Flutter draws, where Back brings a remote's focus.
  final _tabFocus = List.generate(4, (i) => KeyNode(debugLabel: 'tab $i'));

  /// Each tab's page level, for Down from the tabs to land on its first target.
  final _pageLevel = List.generate(4, (_) => GlobalKey<FocusLevelState>());

  /// Down from a tab goes into the page it shows, at its first target, not
  /// to whatever happens to sit below the tab.
  KeyEventResult _barKey(FocusNode _, KeyEvent e) {
    if (e is! KeyDownEvent || e.logicalKey != LogicalKeyboardKey.arrowDown) return KeyEventResult.ignored;
    return _pageLevel[widget.app.tab].currentState?.focusFirst() ?? false ? KeyEventResult.handled : KeyEventResult.ignored;
  }

  @override
  void dispose() {
    for (final n in _tabFocus) {
      n.dispose();
    }
    super.dispose();
  }

  /// Back, up one level. A finger's: a sheet, a 设置 page, an opened card,
  /// the 代理 tab, out. A remote's goes through the levels it moves in: out
  /// of a text field, a sheet, a 设置 page, a route's exits, an opened card,
  /// to the tabs, then the same.
  void _back() {
    final app = widget.app;
    final keys = keyDriven;
    final nav = app.settingsNav.currentState;
    final now = FocusManager.instance.primaryFocus;
    final inBar = _tabFocus.any((n) => n.hasFocus);
    final bar = _tabFocus[app.tab];
    if (keys && now != null && now.context?.findAncestorStateOfType<EditableTextState>() != null) {
      // Android has closed the keyboard already; this leaves the field
      final box = now.ancestors.where((n) => n is! FocusScopeNode && n.canRequestFocus && !n.skipTraversal).firstOrNull;
      box == null ? now.unfocus() : focusTo(box);
    } else if (app.sheet != null) {
      app.closeSheet();
    } else if (app.tab == 3 && nav != null && nav.canPop()) {
      nav.pop();
    } else if (app.tab == 0 && app.x != null) {
      keys && app.sub != null ? app.subBack() : app.collapse();
    } else if (keys && !inBar && bar.context != null && bar.canRequestFocus) {
      focusTo(bar);
    } else if (app.tab != 0) {
      app.selectTab(0);
      if (keys && inBar) focusTo(_tabFocus[0]);
    } else {
      SystemNavigator.pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app;
    final frame = Frame.of(context);
    final kb = MediaQuery.viewInsetsOf(context).bottom;
    final safe = frame.overscan;
    // Each tab's page is a level: only the one shown takes focus, and none
    // while a sheet is over it.
    Widget page(int i, Widget child) => TickerMode(
          enabled: app.tab == i,
          child: FocusLevel(key: _pageLevel[i], active: app.tab == i, covered: app.sheet != null, take: false, child: child),
        );
    // The tabs are there to go to from a page, not from a card or a sheet.
    final barShut = app.sheet != null || (app.tab == 0 && app.x != null);
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _back();
      },
      child: Column(children: [
      Expanded(
        // Full height, so a sheet's scrim reaches over the status bar.
        child: Stack(fit: StackFit.expand, children: [
          Padding(
            // With the bar up top, nothing else keeps the pages off the gesture area.
            padding: EdgeInsets.only(
              left: safe.left,
              right: safe.right,
              top: frame.top + safe.top,
              bottom: (frame.drawnTopBar && kb == 0 ? frame.homeIndicator : 0) + (kb == 0 ? safe.bottom : 0),
            ),
            child: Column(children: [
              if (frame.drawnTopBar) ExcludeFocus(
                  excluding: barShut,
                  child: Focus(canRequestFocus: false, skipTraversal: true, onKeyEvent: _barKey, child: TopTabs(app: app, focus: _tabFocus)),
                ),
              Expanded(
                child: IndexedStack(
                  // a focused row stands out past the page's sides, into the margin
                  clipBehavior: Clip.none,
                  index: app.tab,
                  children: [
                    page(
                      0,
                      Padding(
                        // a keyboard squeezes the opened card rather than covering its footer
                        padding: EdgeInsets.only(bottom: frame.nativeBar ? (kb > frame.barInset ? kb : frame.barReserve) : kb),
                        child: Column(children: [
                          ExcludeFocus(excluding: app.x != null, child: UpdateNotice(app: app)),
                          Expanded(child: Board(app: app)),
                        ]),
                      ),
                    ),
                    page(1, ActivityPage(app: app)),
                    page(2, ProvidersPage(app: app)),
                    page(3, SettingsPage(app: app)),
                  ],
                ),
              ),
            ]),
          ),
          SheetHost(app: app),
          if (app.toast != null) Toast(app.toast!),
        ]),
      ),
      if (!frame.nativeBar && !frame.drawnTopBar && !Platform.isIOS) ExcludeFocus(excluding: barShut, child: AndroidBar(app: app, focus: _tabFocus)),
    ]),
    );
  }
}
