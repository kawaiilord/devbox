import 'package:flutter/material.dart';

/// Shared by the content navigator, so section headers can return to the shell
/// without replacing it. Detail routes use the navigator's normal back button.
class WorkspaceNavigation extends InheritedWidget {
  const WorkspaceNavigation({
    super.key,
    required this.onExit,
    required this.locked,
    required super.child,
  });

  final VoidCallback? onExit;
  final bool locked;

  static Widget? leading(BuildContext context) {
    final scope = context
        .dependOnInheritedWidgetOfExactType<WorkspaceNavigation>();
    if (scope == null || Navigator.of(context).canPop()) return null;
    if (scope.onExit == null) return null;
    return IconButton(
      tooltip: '返回',
      onPressed: scope.locked ? null : scope.onExit,
      icon: const Icon(Icons.arrow_back_rounded),
    );
  }

  @override
  bool updateShouldNotify(WorkspaceNavigation oldWidget) =>
      locked != oldWidget.locked || onExit != oldWidget.onExit;
}

class WorkspaceObserver extends NavigatorObserver {
  WorkspaceObserver(this.onChanged);
  final VoidCallback onChanged;

  @override
  void didChangeTop(Route<dynamic> topRoute, Route<dynamic>? previousTopRoute) {
    // Navigation can be updated while building the shell's page list.
    WidgetsBinding.instance.addPostFrameCallback((_) => onChanged());
  }
}
