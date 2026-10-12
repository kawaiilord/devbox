import 'package:flutter/material.dart';

class WorkspaceObserver extends NavigatorObserver {
  WorkspaceObserver(this.onChanged);
  final VoidCallback onChanged;

  @override
  void didChangeTop(Route<dynamic> topRoute, Route<dynamic>? previousTopRoute) {
    // Navigation can be updated while building the shell's page list.
    WidgetsBinding.instance.addPostFrameCallback((_) => onChanged());
  }
}
