import Flutter
import UIKit

class SceneDelegate: FlutterSceneDelegate {
  private var core: CoreHost?

  override func scene(
    _ scene: UIScene,
    willConnectTo session: UISceneSession,
    options connectionOptions: UIScene.ConnectionOptions
  ) {
    super.scene(scene, willConnectTo: session, options: connectionOptions)
    if let flutter = window?.rootViewController as? FlutterViewController {
      core = CoreHost(messenger: flutter.binaryMessenger)
      window?.rootViewController = ShellController(flutter: flutter)
    }
  }
}
