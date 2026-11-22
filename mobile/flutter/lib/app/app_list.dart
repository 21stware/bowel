/// A list of Android package names, as text to copy between devices and
/// clients: one per line, the form other clients read.
library;

final _package = RegExp(r'[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)+');
final _whole = RegExp('^${_package.pattern}\$');

/// Whether the text is one whole package name, as a rule for an app takes.
bool isPackage(String text) => _whole.hasMatch(text);

/// The package names in pasted text. It takes any separators, and picks the
/// names out of another client's JSON too.
Set<String> parsePackages(String text) => {for (final m in _package.allMatches(text)) m[0]!};

/// The text to copy for these package names.
String packagesText(Iterable<String> ids) => ([...ids.where((s) => s.isNotEmpty)]..sort()).join('\n');
