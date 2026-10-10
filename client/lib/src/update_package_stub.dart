import 'update_service.dart';

Future<String> downloadVerifiedPackage(UpdateManifest manifest) =>
    throw const UpdateException('此平台不支持应用内下载安装包');
