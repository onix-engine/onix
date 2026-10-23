import AVFoundation
import Foundation

final class OnixSystemSpeech: NSObject, AVSpeechSynthesizerDelegate {
	private static let commandSpeak: UInt64 = 1
	private static let commandPause: UInt64 = 2
	private static let commandResume: UInt64 = 3
	private static let commandStop: UInt64 = 4
	private static let eventAvailable: Int32 = 1
	private static let eventStarted: Int32 = 2
	private static let eventCompleted: Int32 = 3
	private static let eventPaused: Int32 = 4
	private static let eventResumed: Int32 = 5
	private static let eventStopped: Int32 = 6
	private static let eventError: Int32 = 7

	private let synthesizer = AVSpeechSynthesizer()
	private var commandSequence: UInt64 = 0
	private var activeUtterance: AVSpeechUtterance?
	private var ignoredCancellation: AVSpeechUtterance?

	override init() {
		super.init()
		synthesizer.delegate = self
		OnixSpeechEvent(Self.eventAvailable)
	}

	func sync() {
		let packed = UInt64(OnixSpeechCommand())
		let sequence = packed >> 8
		guard sequence != 0, sequence != commandSequence else { return }
		commandSequence = sequence
		switch packed & 0xff {
		case Self.commandSpeak: speak()
		case Self.commandPause:
			if synthesizer.pauseSpeaking(at: .word) { OnixSpeechEvent(Self.eventPaused) }
		case Self.commandResume:
			if synthesizer.continueSpeaking() { OnixSpeechEvent(Self.eventResumed) }
		case Self.commandStop:
			ignoredCancellation = activeUtterance
			_ = synthesizer.stopSpeaking(at: .immediate)
			activeUtterance = nil
			OnixSpeechEvent(Self.eventStopped)
		default: break
		}
	}

	func close() {
		ignoredCancellation = activeUtterance
		_ = synthesizer.stopSpeaking(at: .immediate)
		activeUtterance = nil
		synthesizer.delegate = nil
	}

	private func speak() {
		ignoredCancellation = activeUtterance
		_ = synthesizer.stopSpeaking(at: .immediate)
		let utterance = AVSpeechUtterance(string: readString(OnixSpeechText))
		let language = readString(OnixSpeechLanguage)
		let voice = readString(OnixSpeechVoice)
		if !voice.isEmpty {
			utterance.voice = AVSpeechSynthesisVoice(identifier: voice)
		}
		if utterance.voice == nil, !language.isEmpty {
			utterance.voice = AVSpeechSynthesisVoice(language: language)
		}
		let rate = Float(OnixSpeechRate())
		utterance.rate = min(AVSpeechUtteranceMaximumSpeechRate, max(AVSpeechUtteranceMinimumSpeechRate, AVSpeechUtteranceDefaultSpeechRate * rate))
		utterance.pitchMultiplier = Float(OnixSpeechPitch())
		utterance.volume = Float(OnixSpeechVolume())
		activeUtterance = utterance
		synthesizer.speak(utterance)
	}

	private func readString(_ getter: (UnsafeMutablePointer<CChar>?, Int32) -> Int32) -> String {
		let length = Int(getter(nil, 0))
		guard length > 0 else { return "" }
		var bytes = [CChar](repeating: 0, count: length + 1)
		bytes.withUnsafeMutableBufferPointer { pointer in
			_ = getter(pointer.baseAddress, Int32(length))
		}
		return String(cString: bytes)
	}

	func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didStart utterance: AVSpeechUtterance) {
		guard utterance === activeUtterance else { return }
		OnixSpeechEvent(Self.eventStarted)
	}

	func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
		guard utterance === activeUtterance else { return }
		activeUtterance = nil
		OnixSpeechEvent(Self.eventCompleted)
	}

	func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
		if utterance === ignoredCancellation {
			ignoredCancellation = nil
			return
		}
		guard utterance === activeUtterance else { return }
		activeUtterance = nil
		OnixSpeechEvent(Self.eventStopped)
	}
}
