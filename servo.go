package dynamixel

import (
	"errors"
	"fmt"
	"math"
)

const (

	// Control table size (in bytes)
	// TODO: Instead of hard-coding this, maybe calculate the size by finding the
	//       highest register address and adding its length?
	tableSize = 50

	// Control Table Addresses (EEPROM)
	addrID                byte = 0x03 // 1
	addrStatusReturnLevel byte = 0x10 // 1

	// Control Table Addresses (RAM, Read/Write)
	addrGoalPosition byte = 0x1E // 2
	addrTorqueLimit  byte = 0x22 // 2

	// Limits (from dxl_ax_actuator.htm)
	// TODO: Move these to the registValers
	maxPos   uint16  = 1023
	maxSpeed uint16  = 1023
	maxAngle float64 = 300

	// Unit conversions
	positionToAngle float64 = maxAngle / float64(maxPos) // 0.293255132
	angleToPosition float64 = 1 / positionToAngle        // 3.41
)

type DynamixelServo struct {
	Network   Networker
	Ident     uint8
	zeroAngle float64

	// Cache of control table values
	cache [tableSize]byte

	// TODO: Remove this attribute in favor of reading the value from the control
	//       table cache.
	statusReturnLevel int
}

// NewServo returns a new DynamixelServo with its cache populated.
// TODO: Return a pointer, error tuple! We're currently ignoring the return
//       value of the updateCache call.
func NewServo(network Networker, ident uint8) *DynamixelServo {
	s := &DynamixelServo{
		Network:           network,
		Ident:             ident,
		zeroAngle:         150,
		statusReturnLevel: 2,
	}

	_ = s.updateCache()
	return s
}

// updateCache reads the entire control table from the servo, and stores it in
// the cache.
func (servo *DynamixelServo) updateCache() error {
	b, err := servo.Network.ReadData(servo.Ident, 0x0, tableSize)
	if err != nil {
		return err
	}

	// Ensure that the returned slice is the right size.
	if len(b) != tableSize {
		return fmt.Errorf("invalid control table size: %d (expected %d)", len(b), tableSize)
	}

	// Copy each byte to the cache.
	// TODO: Surely there is a better way to do this.
	for i := 0; i < tableSize; i++ {
		servo.cache[i] = b[i]
	}

	return nil
}

// getRegister fetches the value of a register from the cache.
func (servo *DynamixelServo) getRegister(reg Register) (int, error) {
	if reg.length != 1 && reg.length != 2 {
		return 0, fmt.Errorf("invalid register length: %d", reg.length)
	}

	if reg.cacheable {
		v := int(servo.cache[reg.address])

		if reg.length == 2 {
			v |= int(servo.cache[reg.address+1]) << 8
		}

		return v, nil
	} else {
		if servo.statusReturnLevel == 0 {
			return 0, errors.New("can't READ while Status Return Level is zero")
		}

		b, err := servo.Network.ReadData(servo.Ident, reg.address, reg.length)
		if err != nil {
			return 0, err
		}

		switch len(b) {
		case 1:
			servo.cache[reg.address] = b[0]
			return int(b[0]), nil

		case 2:
			servo.cache[reg.address] = b[0]
			servo.cache[reg.address+1] = b[1]
			return int(b[0]) | int(b[1])<<8, nil

		default:
			return 0, fmt.Errorf("expected %d bytes, got %d", reg.length, len(b))

		}
	}
}

// setRegister writes a value to the given register. Returns an error if the
// register is read only or if the write failed.
func (servo *DynamixelServo) setRegister(reg Register, value int) error {
	if reg.access == ro {
		return fmt.Errorf("can't write to a read-only register")
	}

	if value < reg.min {
		return fmt.Errorf("value too low: %d (min=%d)", value, reg.min)
	}

	if value > reg.max {
		return fmt.Errorf("value too high: %d (max=%d)", value, reg.max)
	}

	// TODO: Add log message when setting a register.
	switch reg.length {
	case 1:
		servo.writeData(reg.address, low(value))
		servo.cache[reg.address] = low(value)

	case 2:
		servo.writeData(reg.address, low(value), high(value))
		servo.cache[reg.address] = low(value)
		servo.cache[reg.address+1] = high(value)

	default:
		return fmt.Errorf("invalid register length: %d", reg.length)
	}

	return nil
}

// Ping sends the PING instruction to servo, and waits for the response. Returns
// nil if the ping succeeds, otherwise an error. It's optional, but a very good
// idea, to call this before sending any other instructions to the servo.
func (servo *DynamixelServo) Ping() error {
	return servo.Network.Ping(servo.Ident)
}

func (servo *DynamixelServo) readInt(addr byte, n int) (int, error) {
	if servo.statusReturnLevel == 0 {
		return 0, errors.New("can't READ while Status Return Level is zero")
	}

	b, err := servo.Network.ReadData(servo.Ident, addr, n)
	if err != nil {
		return 0, err
	}

	return bytesToInt(b)
}

// TODO: Remove this in favor of setRegister?
func (servo *DynamixelServo) writeData(params ...byte) error {
	return servo.Network.WriteData(servo.Ident, (servo.statusReturnLevel == 2), params...)
}

func posDistance(a uint16, b uint16) uint16 {
	return uint16(math.Abs(float64(a) - float64(b)))
}

//
func normalizeAngle(d float64) float64 {
	if d > 180 {
		return normalizeAngle(d - 360)

	} else if d < -180 {
		return normalizeAngle(d + 360)

	} else {
		return d
	}
}

//
// -- Registers
//
//    These methods are getters for the various registers in the control table.
//    Some of them (where register.cacheable == true) just read from the cache,
//    while others read the actual control table every time.
//
// TODO: Each of the following registers should have a corresponding reader, and
//       the R/W registers (marked with an asterisk) should have a writer. They
//       should all receive and return ints or bools, rather than bytes.
//
// TODO: These methods should probably be generated from the list of registers,
//       especially if/when we support multiple models with different sets.
///
// modelNumber
// firmwareVersion
// servoID*
// baudRate*
// returnDelayTime*
// cwAngleLimit*
// ccwAngleLimit*
// highestLimitTemperature*
// lowestLimitVoltage*
// highestLimitVoltage*
// maxTorque*
// statusReturnLevel*
// alarmLed*
// alarmShutdown*
// torqueEnable*
// led*
// cwComplianceMargin*
// ccwComplianceMargin*
// cwComplianceSlope*
// ccwComplianceSlope*
// goalPosition*
// movingSpeed*
// torqueLimit*
// presentPosition
// presentSpeed
// presentLoad
// presentVoltage
// presentTemperature
// registered
// moving
// lock*
// punch*
//

func (servo *DynamixelServo) ModelNumber() (int, error) {
	return servo.getRegister(*registers[modelNumber])
}

func (servo *DynamixelServo) FirmwareVersion() (int, error) {
	return servo.getRegister(*registers[firmwareVersion])
}

func (servo *DynamixelServo) PresentSpeed() (int, error) {
	return servo.getRegister(*registers[presentSpeed])
}

func (servo *DynamixelServo) TorqueEnable() (bool, error) {
	v, err := servo.getRegister(*registers[torqueEnable])
	return itob(v), err
}

// Enables or disables torque.
func (servo *DynamixelServo) SetTorqueEnable(state bool) error {
	servo.logMethod("SetTorqueEnable(%t)", state)
	return servo.setRegister(*registers[torqueEnable], btoi(state))
}

// LED returns the current state of the servo's LED.
// TODO: Should we continue to return bool here, or expose the int?
func (servo *DynamixelServo) LED() (bool, error) {
	v, err := servo.getRegister(*registers[led])
	return itob(v), err
}

// Enables or disables the servo's LED.
func (servo *DynamixelServo) SetLED(state bool) error {
	return servo.setRegister(*registers[led], btoi(state))
}

func (servo *DynamixelServo) GoalPosition() (int, error) {
	return servo.getRegister(*registers[goalPosition])
}

// SetGoalPosition sets the goal position.
func (servo *DynamixelServo) SetGoalPosition(pos int) error {

	// TODO: Reject if the servo is in wheel mode (where CW and CCW angle limit
	//       is zero).

	reg := *registers[goalPosition]

	if pos < reg.min || pos > reg.max {
		return errors.New("goal position out of range")
	}

	return servo.setRegister(reg, pos)
}

// MovingSpeed returns the current moving speed. This is not the speed at which
// the motor is moving, it's the speed at which the servo wants to move.
func (servo *DynamixelServo) MovingSpeed() (int, error) {
	return servo.getRegister(*registers[movingSpeed])
}

// Sets the moving speed.
func (servo *DynamixelServo) SetMovingSpeed(speed int) error {
	if speed < 0 || speed > int(maxSpeed) {
		return errors.New("moving speed out of range")
	}

	return servo.setRegister(*registers[movingSpeed], speed)
}

func (servo *DynamixelServo) PresentPosition() (int, error) {
	return servo.getRegister(*registers[presentPosition])
}

func (servo *DynamixelServo) PresentVoltage() (int, error) {
	return servo.getRegister(*registers[presentVoltage])
}

func (servo *DynamixelServo) PresentLoad() (int, error) {
	return servo.getRegister(*registers[presentLoad])
}

func (servo *DynamixelServo) PresentTemperature() (int, error) {
	return servo.getRegister(*registers[presentTemperature])
}

func (servo *DynamixelServo) Registered() (int, error) {
	return servo.getRegister(*registers[registered])
}

func (servo *DynamixelServo) Moving() (int, error) {
	return servo.getRegister(*registers[moving])
}

// TODO: Rename this to avoid confusion?
func (servo *DynamixelServo) Lock() (int, error) {
	return servo.getRegister(*registers[lock])
}

func (servo *DynamixelServo) SetLock(isLocked int) error {
	if isLocked > 1 || isLocked < 0 {
		return errors.New("invalid lock value (must be zero or one)")
	}

	// Can't unlock when servo is locked, so if we know that's the case, don't
	// bother trying. Can be overriden by clearing the cache.
	//
	// TODO: Add a method to read ints from the cache. If we used getRegister,
	//       we risk accidentally (in the case of a bug) reading from the actual
	//       device, which would be slow and weird.

	reg := *registers[lock]

	if isLocked == 0 && servo.cache[reg.address] == byte(1) {
		return errors.New("EEPROM can't be unlocked; must be power-cycled")
	}

	return servo.setRegister(reg, isLocked)
}

//
// -- High-level interface
//
//    These methods should provide as useful and friendly of an interface to the
//    servo as possible.

func (servo *DynamixelServo) posToAngle(pos int) float64 {
	return (positionToAngle * float64(pos)) - servo.zeroAngle
}

func (servo *DynamixelServo) angleToPos(angle float64) int {
	return int((servo.zeroAngle + angle) * angleToPosition)
}

// Sets the origin angle (in degrees).
func (servo *DynamixelServo) SetZero(offset float64) {
	servo.zeroAngle = offset
}

//
// Returns the current position of the servo, relative to the zero angle.
//
func (servo *DynamixelServo) Angle() (float64, error) {
	pos, err := servo.Position()

	if err != nil {
		return 0, err

	} else {
		return servo.posToAngle(pos), nil
	}
}

// MoveTo sets the goal position of the servo by angle (in degrees), where zero
// is the midpoint, 150 deg is max left (clockwise), and -150 deg is max right
// (counter-clockwise). This is generally preferable to calling SetGoalPosition,
// which uses the internal uint16 representation.
//
// If the angle is out of bounds
//
func (servo *DynamixelServo) MoveTo(angle float64) error {
	pos := servo.angleToPos(normalizeAngle(angle))
	return servo.SetGoalPosition(pos)
}

//
// -- Low-level interface
//
//    These methods should follow the Dynamixel protocol docs as closely as
//    possible, with no fancy stuff.
//

// Sets the torque limit.
func (servo *DynamixelServo) SetTorqueLimit(limit int) error {
	servo.logMethod("SetTorqueLimit(%d)", limit)

	if limit < 0 || limit > 1023 {
		return errors.New("torque limit out of range")
	}
	return servo.writeData(addrTorqueLimit, low(limit), high(limit))
}

// Sets the status return level. Possible values are:
//
// 0 = Only respond to PING commands
// 1 = Only respond to PING and READ commands
// 2 = Respond to all commands
//
// Servos default to 2, but retain the value so long as they're powered up. This
// makes it a very good idea to explicitly set the value after connecting, to
// avoid waiting for status packets which will never arrive.
//
// See: dxl_ax_actuator.htm#Actuator_Address_10
func (servo *DynamixelServo) SetStatusReturnLevel(value int) error {
	servo.logMethod("SetStatusReturnLevel(%d)", value)

	if value < 0 || value > 2 {
		return fmt.Errorf("invalid Status Return Level value: %d", value)
	}

	// Call Network.WriteData directly, rather than via servo.writeData, because
	// the return status level will depend upon the new level, rather than the
	// current level cache. We don't want to update that until we're sure that
	// the write was successful.
	err := servo.Network.WriteData(servo.Ident, (value == 2), addrStatusReturnLevel, low(value))
	if err != nil {
		return err
	}

	servo.statusReturnLevel = value
	return nil
}

// Changes the identity of the servo.
// This is stored in EEPROM, so will persist between reboots.
func (servo *DynamixelServo) SetIdent(ident int) error {
	servo.logMethod("SetIdent(%d, %d)", ident)
	i := low(ident)

	if i < 0 || i > 252 {
		return fmt.Errorf("invalid ID (must be 0-252): %d", i)
	}

	err := servo.writeData(addrID, i)
	if err != nil {
		return err
	}

	servo.Ident = i
	return nil
}

// Voltage returns the current voltage supplied. Unlike the underlying register,
// this is the actual voltage, not multiplied by ten.
func (servo *DynamixelServo) Voltage() (float64, error) {
	val, err := servo.PresentVoltage()
	if err != nil {
		return 0.0, err
	}

	// Convert the return value into actual volts.
	return (float64(val) / 10), nil
}

// Position is an alias for PresentPosition.
func (servo *DynamixelServo) Position() (int, error) {
	return servo.PresentPosition()
}

func (servo *DynamixelServo) logMethod(format string, v ...interface{}) {
	prefix := fmt.Sprintf("servo[%d].", servo.Ident)
	servo.Network.Log(prefix+format, v...)
}
