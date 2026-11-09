import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import 'frame.dart';

/// The scrim and panel of the sheet that's up. It's drawn inside the page
/// area, because iOS's tab bar is a native view over all of Flutter.
class SheetHost extends StatelessWidget {
  const SheetHost({super.key, required this.app});
  final AppState app;

  @override
  Widget build(BuildContext context) {
    final build = app.sheet;
    return AnimatedSwitcher(
      duration: const Duration(milliseconds: 200),
      child: build == null ? const SizedBox.shrink(key: ValueKey('none')) : _Panel(key: ValueKey(app.sheetId), app: app, child: Builder(builder: build)),
    );
  }
}

class _Panel extends StatelessWidget {
  const _Panel({super.key, required this.app, required this.child});
  final AppState app;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final f = Frame.of(context);
    final kb = MediaQuery.viewInsetsOf(context).bottom;
    final bottom = f.nativeBar ? (kb > f.barInset ? kb + 6 : f.barReserve) : (kb > 0 ? kb + 6 : 8.0);
    return Stack(fit: StackFit.expand, children: [
      GestureDetector(onTap: app.closeSheet, child: const ColoredBox(color: Color(0x66000000))),
      Align(
        alignment: Alignment.bottomCenter,
        child: TweenAnimationBuilder<double>(
          tween: Tween(begin: 40, end: 0),
          duration: const Duration(milliseconds: 280),
          curve: easeCard,
          builder: (context, dy, child) => Transform.translate(offset: Offset(0, dy), child: child),
          child: Padding(
            padding: EdgeInsets.fromLTRB(8, 40, 8, bottom),
            child: ConstrainedBox(
              constraints: BoxConstraints(maxWidth: f.form == WindowForm.phone ? double.infinity : 520, maxHeight: (MediaQuery.sizeOf(context).height - f.top - bottom - 60) * .86),
              child: Container(
                padding: const EdgeInsets.fromLTRB(16, 14, 16, 12),
                decoration: BoxDecoration(color: l.card, borderRadius: l.br(18), border: Border.all(color: l.hair)),
                child: child,
              ),
            ),
          ),
        ),
      ),
    ]);
  }
}

class PickOption<T> {
  const PickOption(this.value, this.title, {this.sub, this.danger = false, this.leading});
  final T value;
  final String title;
  final String? sub;
  final bool danger;
  final Widget? leading;
}

/// A list to pick one from. Long lists get a search.
void pickSheet<T>(AppState app, String title, List<PickOption<T>> options, void Function(T) onPick, {T? value, bool search = false}) {
  app.showSheet((_) => _PickList<T>(app: app, title: title, options: options, onPick: onPick, value: value, search: search));
}

class _PickList<T> extends StatefulWidget {
  const _PickList({super.key, required this.app, required this.title, required this.options, required this.onPick, this.value, this.search = false});

  final AppState app;
  final String title;
  final List<PickOption<T>> options;
  final void Function(T) onPick;
  final T? value;
  final bool search;

  @override
  State<_PickList<T>> createState() => _PickListState<T>();
}

class _PickListState<T> extends State<_PickList<T>> {
  String _q = '';

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final q = _q.trim().toLowerCase();
    final shown = [for (final o in widget.options) if (q.isEmpty || '${o.title} ${o.sub ?? ''}'.toLowerCase().contains(q)) o];
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(widget.title, style: l.text(15, weight: 700, lh: 21)),
      const SizedBox(height: 6),
      Flexible(
        child: ListView(shrinkWrap: true, padding: EdgeInsets.zero, keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag, children: [
          for (final o in shown)
            Tap(
              onTap: () {
                widget.app.closeSheet();
                widget.onPick(o.value);
              },
              child: Container(
                constraints: const BoxConstraints(minHeight: 48),
                padding: const EdgeInsets.symmetric(vertical: 6),
                decoration: BoxDecoration(border: Border(bottom: BorderSide(color: l.hair2))),
                child: Row(children: [
                  if (o.leading != null) ...[o.leading!, const SizedBox(width: 10)],
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      Text(o.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: l.text(15, color: o.danger ? l.signalT : l.ink, lh: 21)),
                      if (o.sub != null) Text(o.sub!, maxLines: 2, overflow: TextOverflow.ellipsis, style: l.text(12.5, color: l.mute, lh: 17)),
                    ]),
                  ),
                  if (o.value == widget.value) Ic('check', size: 18, color: l.ink),
                ]),
              ),
            ),
        ]),
      ),
      if (widget.search) ...[const SizedBox(height: 8), SearchField(hint: tr('搜索', 'Search'), onChanged: (v) => setState(() => _q = v))],
    ]);
  }
}

/// Asks for a line, or several, of text.
void inputSheet(AppState app, String title, void Function(String) onSave, {String? hint, String initial = '', String? note, String? action, int? maxLines = 1, TextInputType? keyboard}) {
  app.showSheet((_) => _Input(app: app, title: title, onSave: onSave, hint: hint ?? '', initial: initial, note: note, action: action ?? tr('保存', 'Save'), maxLines: maxLines, keyboard: keyboard));
}

class _Input extends StatefulWidget {
  const _Input({required this.app, required this.title, required this.onSave, required this.hint, required this.initial, this.note, required this.action, this.maxLines, this.keyboard});

  final AppState app;
  final String title, hint, initial, action;
  final String? note;
  final void Function(String) onSave;
  final int? maxLines;
  final TextInputType? keyboard;

  @override
  State<_Input> createState() => _InputState();
}

class _InputState extends State<_Input> {
  late final _c = TextEditingController(text: widget.initial);

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  void _save() {
    widget.app.closeSheet();
    widget.onSave(_c.text.trim());
  }

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final many = widget.maxLines != 1;
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(widget.title, style: l.text(15, weight: 700, lh: 21)),
      if (widget.note != null) Padding(padding: const EdgeInsets.only(top: 2), child: Text(widget.note!, style: l.text(12.5, color: l.mute, lh: 17))),
      const SizedBox(height: 10),
      Flexible(
        child: Container(
          constraints: BoxConstraints(minHeight: many ? 120 : 44),
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
          decoration: BoxDecoration(color: l.paper2, borderRadius: l.br(11)),
          child: BareField(hint: widget.hint, controller: _c, maxLines: widget.maxLines, autofocus: true, keyboardType: many ? TextInputType.multiline : widget.keyboard, onSubmitted: many ? null : (_) => _save()),
        ),
      ),
      const SizedBox(height: 10),
      Btn(widget.action, kind: BtnKind.accent, expand: true, minH: 44, onTap: _save),
    ]);
  }
}

/// Makes sure about something that can't be taken back.
void confirmSheet(AppState app, String title, String action, VoidCallback onYes, {String? body}) {
  app.showSheet((context) {
    final l = Skin.of(context);
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(title, style: l.text(15, weight: 700, lh: 21)),
      if (body != null) Padding(padding: const EdgeInsets.only(top: 4), child: Text(body, style: l.text(13.5, color: l.ink2, lh: 19))),
      const SizedBox(height: 14),
      Row(children: [
        Expanded(child: Btn(tr('取消', 'Cancel'), expand: true, minH: 44, onTap: app.closeSheet)),
        const SizedBox(width: 8),
        Expanded(
          child: Btn(action, kind: BtnKind.accent, expand: true, minH: 44, onTap: () {
            app.closeSheet();
            onYes();
          }),
        ),
      ]),
    ]);
  });
}
