/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-mixed-operators, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars, default-case, jsdoc/require-param*/
"use strict";

var $protobuf = require("protobufjs/minimal");

// Common aliases
var $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;
var $Object = $util.global.Object, $undefined = $util.global.undefined, $Error = $util.global.Error, $RangeError = $util.global.RangeError, $Array = $util.global.Array;

// Exported root namespace
var $root = $protobuf.roots["minimir_v1_candidate"] || ($protobuf.roots["minimir_v1_candidate"] = {});

$root.minimir = (function() {

    /**
     * Namespace minimir.
     * @exports minimir
     * @namespace
     */
    var minimir = {};

    minimir.v1 = (function() {

        /**
         * Namespace v1.
         * @memberof minimir
         * @namespace
         */
        var v1 = {};

        v1.RoleKey = (function() {

            /**
             * Properties of a RoleKey.
             * @typedef {Object} minimir.v1.RoleKey.$Properties
             * @property {Uint8Array|null} [role_id] RoleKey role_id
             * @property {Long|null} [origin_server_id] RoleKey origin_server_id
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a RoleKey.
             * @memberof minimir.v1
             * @interface IRoleKey
             * @augments minimir.v1.RoleKey.$Properties
             * @deprecated Use minimir.v1.RoleKey.$Properties instead.
             */

            /**
             * Shape of a RoleKey.
             * @typedef {minimir.v1.RoleKey.$Properties} minimir.v1.RoleKey.$Shape
             */

            /**
             * Constructs a new RoleKey.
             * @memberof minimir.v1
             * @classdesc Represents a RoleKey.
             * @constructor
             * @param {minimir.v1.RoleKey.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var RoleKey = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * RoleKey role_id.
             * @member {Uint8Array} role_id
             * @memberof minimir.v1.RoleKey
             * @instance
             */
            RoleKey.prototype.role_id = $util.newBuffer([]);

            /**
             * RoleKey origin_server_id.
             * @member {Long} origin_server_id
             * @memberof minimir.v1.RoleKey
             * @instance
             */
            RoleKey.prototype.origin_server_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new RoleKey instance using the specified properties.
             * @function create
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {minimir.v1.RoleKey.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.RoleKey} RoleKey instance
             * @type {{
             *   (properties: minimir.v1.RoleKey.$Shape): minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape;
             *   (properties?: minimir.v1.RoleKey.$Properties): minimir.v1.RoleKey;
             * }}
             */
            RoleKey.create = function(properties) {
                return new RoleKey(properties);
            };

            /**
             * Encodes the specified RoleKey message. Does not implicitly {@link minimir.v1.RoleKey.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {minimir.v1.RoleKey.$Properties} message RoleKey message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleKey.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.role_id != null && $Object.hasOwnProperty.call(message, "role_id") && message.role_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.role_id);
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id") && (typeof message.origin_server_id === "object" ? message.origin_server_id.low || message.origin_server_id.high : message.origin_server_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.origin_server_id);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified RoleKey message, length delimited. Does not implicitly {@link minimir.v1.RoleKey.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {minimir.v1.RoleKey.$Properties} message RoleKey message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleKey.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a RoleKey message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape} RoleKey
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleKey.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.RoleKey();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.role_id = value;
                            else
                                delete message.role_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.origin_server_id = value;
                            else
                                delete message.origin_server_id;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a RoleKey message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleKey & minimir.v1.RoleKey.$Shape} RoleKey
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleKey.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a RoleKey message.
             * @function verify
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            RoleKey.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.role_id != null && $Object.hasOwnProperty.call(message, "role_id"))
                    if (!(message.role_id && typeof message.role_id.length === "number" || $util.isString(message.role_id)))
                        return "role_id: buffer expected";
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id"))
                    if (!$util.isInteger(message.origin_server_id) && !(message.origin_server_id && $util.isInteger(message.origin_server_id.low) && $util.isInteger(message.origin_server_id.high)))
                        return "origin_server_id: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for RoleKey
             * @function getTypeUrl
             * @memberof minimir.v1.RoleKey
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            RoleKey.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.RoleKey";
            };

            return RoleKey;
        })();

        v1.GridPoint = (function() {

            /**
             * Properties of a GridPoint.
             * @typedef {Object} minimir.v1.GridPoint.$Properties
             * @property {number|null} [x] GridPoint x
             * @property {number|null} [y] GridPoint y
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a GridPoint.
             * @memberof minimir.v1
             * @interface IGridPoint
             * @augments minimir.v1.GridPoint.$Properties
             * @deprecated Use minimir.v1.GridPoint.$Properties instead.
             */

            /**
             * Shape of a GridPoint.
             * @typedef {minimir.v1.GridPoint.$Properties} minimir.v1.GridPoint.$Shape
             */

            /**
             * Constructs a new GridPoint.
             * @memberof minimir.v1
             * @classdesc Represents a GridPoint.
             * @constructor
             * @param {minimir.v1.GridPoint.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var GridPoint = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * GridPoint x.
             * @member {number} x
             * @memberof minimir.v1.GridPoint
             * @instance
             */
            GridPoint.prototype.x = 0;

            /**
             * GridPoint y.
             * @member {number} y
             * @memberof minimir.v1.GridPoint
             * @instance
             */
            GridPoint.prototype.y = 0;

            /**
             * Creates a new GridPoint instance using the specified properties.
             * @function create
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {minimir.v1.GridPoint.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.GridPoint} GridPoint instance
             * @type {{
             *   (properties: minimir.v1.GridPoint.$Shape): minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape;
             *   (properties?: minimir.v1.GridPoint.$Properties): minimir.v1.GridPoint;
             * }}
             */
            GridPoint.create = function(properties) {
                return new GridPoint(properties);
            };

            /**
             * Encodes the specified GridPoint message. Does not implicitly {@link minimir.v1.GridPoint.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {minimir.v1.GridPoint.$Properties} message GridPoint message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            GridPoint.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.x != null && $Object.hasOwnProperty.call(message, "x") && message.x !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).sint32(message.x);
                if (message.y != null && $Object.hasOwnProperty.call(message, "y") && message.y !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).sint32(message.y);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified GridPoint message, length delimited. Does not implicitly {@link minimir.v1.GridPoint.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {minimir.v1.GridPoint.$Properties} message GridPoint message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            GridPoint.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a GridPoint message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape} GridPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            GridPoint.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.GridPoint();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.sint32())
                                message.x = value;
                            else
                                delete message.x;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.sint32())
                                message.y = value;
                            else
                                delete message.y;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a GridPoint message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.GridPoint & minimir.v1.GridPoint.$Shape} GridPoint
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            GridPoint.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a GridPoint message.
             * @function verify
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            GridPoint.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.x != null && $Object.hasOwnProperty.call(message, "x"))
                    if (!$util.isInteger(message.x))
                        return "x: integer expected";
                if (message.y != null && $Object.hasOwnProperty.call(message, "y"))
                    if (!$util.isInteger(message.y))
                        return "y: integer expected";
                return null;
            };

            /**
             * Gets the type url for GridPoint
             * @function getTypeUrl
             * @memberof minimir.v1.GridPoint
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            GridPoint.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.GridPoint";
            };

            return GridPoint;
        })();

        v1.OperationContext = (function() {

            /**
             * Properties of an OperationContext.
             * @typedef {Object} minimir.v1.OperationContext.$Properties
             * @property {Uint8Array|null} [op_id] OperationContext op_id
             * @property {Long|null} [command_seq] OperationContext command_seq
             * @property {Long|null} [expected_revision] OperationContext expected_revision
             * @property {Long|null} [expected_rule_version] OperationContext expected_rule_version
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an OperationContext.
             * @memberof minimir.v1
             * @interface IOperationContext
             * @augments minimir.v1.OperationContext.$Properties
             * @deprecated Use minimir.v1.OperationContext.$Properties instead.
             */

            /**
             * Shape of an OperationContext.
             * @typedef {minimir.v1.OperationContext.$Properties} minimir.v1.OperationContext.$Shape
             */

            /**
             * Constructs a new OperationContext.
             * @memberof minimir.v1
             * @classdesc Represents an OperationContext.
             * @constructor
             * @param {minimir.v1.OperationContext.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var OperationContext = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * OperationContext op_id.
             * @member {Uint8Array} op_id
             * @memberof minimir.v1.OperationContext
             * @instance
             */
            OperationContext.prototype.op_id = $util.newBuffer([]);

            /**
             * OperationContext command_seq.
             * @member {Long} command_seq
             * @memberof minimir.v1.OperationContext
             * @instance
             */
            OperationContext.prototype.command_seq = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * OperationContext expected_revision.
             * @member {Long} expected_revision
             * @memberof minimir.v1.OperationContext
             * @instance
             */
            OperationContext.prototype.expected_revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * OperationContext expected_rule_version.
             * @member {Long} expected_rule_version
             * @memberof minimir.v1.OperationContext
             * @instance
             */
            OperationContext.prototype.expected_rule_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new OperationContext instance using the specified properties.
             * @function create
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {minimir.v1.OperationContext.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.OperationContext} OperationContext instance
             * @type {{
             *   (properties: minimir.v1.OperationContext.$Shape): minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape;
             *   (properties?: minimir.v1.OperationContext.$Properties): minimir.v1.OperationContext;
             * }}
             */
            OperationContext.create = function(properties) {
                return new OperationContext(properties);
            };

            /**
             * Encodes the specified OperationContext message. Does not implicitly {@link minimir.v1.OperationContext.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {minimir.v1.OperationContext.$Properties} message OperationContext message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            OperationContext.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id") && message.op_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.op_id);
                if (message.command_seq != null && $Object.hasOwnProperty.call(message, "command_seq") && (typeof message.command_seq === "object" ? message.command_seq.low || message.command_seq.high : message.command_seq !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.command_seq);
                if (message.expected_revision != null && $Object.hasOwnProperty.call(message, "expected_revision") && (typeof message.expected_revision === "object" ? message.expected_revision.low || message.expected_revision.high : message.expected_revision !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.expected_revision);
                if (message.expected_rule_version != null && $Object.hasOwnProperty.call(message, "expected_rule_version") && (typeof message.expected_rule_version === "object" ? message.expected_rule_version.low || message.expected_rule_version.high : message.expected_rule_version !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).uint64(message.expected_rule_version);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified OperationContext message, length delimited. Does not implicitly {@link minimir.v1.OperationContext.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {minimir.v1.OperationContext.$Properties} message OperationContext message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            OperationContext.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an OperationContext message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape} OperationContext
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            OperationContext.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.OperationContext();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.op_id = value;
                            else
                                delete message.op_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.command_seq = value;
                            else
                                delete message.command_seq;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.expected_revision = value;
                            else
                                delete message.expected_revision;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.expected_rule_version = value;
                            else
                                delete message.expected_rule_version;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an OperationContext message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.OperationContext & minimir.v1.OperationContext.$Shape} OperationContext
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            OperationContext.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an OperationContext message.
             * @function verify
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            OperationContext.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id"))
                    if (!(message.op_id && typeof message.op_id.length === "number" || $util.isString(message.op_id)))
                        return "op_id: buffer expected";
                if (message.command_seq != null && $Object.hasOwnProperty.call(message, "command_seq"))
                    if (!$util.isInteger(message.command_seq) && !(message.command_seq && $util.isInteger(message.command_seq.low) && $util.isInteger(message.command_seq.high)))
                        return "command_seq: integer|Long expected";
                if (message.expected_revision != null && $Object.hasOwnProperty.call(message, "expected_revision"))
                    if (!$util.isInteger(message.expected_revision) && !(message.expected_revision && $util.isInteger(message.expected_revision.low) && $util.isInteger(message.expected_revision.high)))
                        return "expected_revision: integer|Long expected";
                if (message.expected_rule_version != null && $Object.hasOwnProperty.call(message, "expected_rule_version"))
                    if (!$util.isInteger(message.expected_rule_version) && !(message.expected_rule_version && $util.isInteger(message.expected_rule_version.low) && $util.isInteger(message.expected_rule_version.high)))
                        return "expected_rule_version: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for OperationContext
             * @function getTypeUrl
             * @memberof minimir.v1.OperationContext
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            OperationContext.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.OperationContext";
            };

            return OperationContext;
        })();

        v1.OperationResult = (function() {

            /**
             * Properties of an OperationResult.
             * @typedef {Object} minimir.v1.OperationResult.$Properties
             * @property {Uint8Array|null} [op_id] OperationResult op_id
             * @property {Long|null} [command_seq] OperationResult command_seq
             * @property {Long|null} [new_revision] OperationResult new_revision
             * @property {number|null} [error_code] OperationResult error_code
             * @property {boolean|null} [pending] OperationResult pending
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an OperationResult.
             * @memberof minimir.v1
             * @interface IOperationResult
             * @augments minimir.v1.OperationResult.$Properties
             * @deprecated Use minimir.v1.OperationResult.$Properties instead.
             */

            /**
             * Shape of an OperationResult.
             * @typedef {minimir.v1.OperationResult.$Properties} minimir.v1.OperationResult.$Shape
             */

            /**
             * Constructs a new OperationResult.
             * @memberof minimir.v1
             * @classdesc Represents an OperationResult.
             * @constructor
             * @param {minimir.v1.OperationResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var OperationResult = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * OperationResult op_id.
             * @member {Uint8Array} op_id
             * @memberof minimir.v1.OperationResult
             * @instance
             */
            OperationResult.prototype.op_id = $util.newBuffer([]);

            /**
             * OperationResult command_seq.
             * @member {Long} command_seq
             * @memberof minimir.v1.OperationResult
             * @instance
             */
            OperationResult.prototype.command_seq = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * OperationResult new_revision.
             * @member {Long} new_revision
             * @memberof minimir.v1.OperationResult
             * @instance
             */
            OperationResult.prototype.new_revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * OperationResult error_code.
             * @member {number} error_code
             * @memberof minimir.v1.OperationResult
             * @instance
             */
            OperationResult.prototype.error_code = 0;

            /**
             * OperationResult pending.
             * @member {boolean} pending
             * @memberof minimir.v1.OperationResult
             * @instance
             */
            OperationResult.prototype.pending = false;

            /**
             * Creates a new OperationResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {minimir.v1.OperationResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.OperationResult} OperationResult instance
             * @type {{
             *   (properties: minimir.v1.OperationResult.$Shape): minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape;
             *   (properties?: minimir.v1.OperationResult.$Properties): minimir.v1.OperationResult;
             * }}
             */
            OperationResult.create = function(properties) {
                return new OperationResult(properties);
            };

            /**
             * Encodes the specified OperationResult message. Does not implicitly {@link minimir.v1.OperationResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {minimir.v1.OperationResult.$Properties} message OperationResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            OperationResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id") && message.op_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.op_id);
                if (message.command_seq != null && $Object.hasOwnProperty.call(message, "command_seq") && (typeof message.command_seq === "object" ? message.command_seq.low || message.command_seq.high : message.command_seq !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.command_seq);
                if (message.new_revision != null && $Object.hasOwnProperty.call(message, "new_revision") && (typeof message.new_revision === "object" ? message.new_revision.low || message.new_revision.high : message.new_revision !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.new_revision);
                if (message.error_code != null && $Object.hasOwnProperty.call(message, "error_code") && message.error_code !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.error_code);
                if (message.pending != null && $Object.hasOwnProperty.call(message, "pending") && message.pending !== false)
                    writer.uint32(/* id 5, wireType 0 =*/40).bool(message.pending);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified OperationResult message, length delimited. Does not implicitly {@link minimir.v1.OperationResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {minimir.v1.OperationResult.$Properties} message OperationResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            OperationResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an OperationResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape} OperationResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            OperationResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.OperationResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.op_id = value;
                            else
                                delete message.op_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.command_seq = value;
                            else
                                delete message.command_seq;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.new_revision = value;
                            else
                                delete message.new_revision;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.error_code = value;
                            else
                                delete message.error_code;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.pending = value;
                            else
                                delete message.pending;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an OperationResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.OperationResult & minimir.v1.OperationResult.$Shape} OperationResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            OperationResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an OperationResult message.
             * @function verify
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            OperationResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id"))
                    if (!(message.op_id && typeof message.op_id.length === "number" || $util.isString(message.op_id)))
                        return "op_id: buffer expected";
                if (message.command_seq != null && $Object.hasOwnProperty.call(message, "command_seq"))
                    if (!$util.isInteger(message.command_seq) && !(message.command_seq && $util.isInteger(message.command_seq.low) && $util.isInteger(message.command_seq.high)))
                        return "command_seq: integer|Long expected";
                if (message.new_revision != null && $Object.hasOwnProperty.call(message, "new_revision"))
                    if (!$util.isInteger(message.new_revision) && !(message.new_revision && $util.isInteger(message.new_revision.low) && $util.isInteger(message.new_revision.high)))
                        return "new_revision: integer|Long expected";
                if (message.error_code != null && $Object.hasOwnProperty.call(message, "error_code"))
                    if (!$util.isInteger(message.error_code))
                        return "error_code: integer expected";
                if (message.pending != null && $Object.hasOwnProperty.call(message, "pending"))
                    if (typeof message.pending !== "boolean")
                        return "pending: boolean expected";
                return null;
            };

            /**
             * Gets the type url for OperationResult
             * @function getTypeUrl
             * @memberof minimir.v1.OperationResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            OperationResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.OperationResult";
            };

            return OperationResult;
        })();

        v1.Heartbeat = (function() {

            /**
             * Properties of a Heartbeat.
             * @typedef {Object} minimir.v1.Heartbeat.$Properties
             * @property {Long|null} [client_monotonic_ms] Heartbeat client_monotonic_ms
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a Heartbeat.
             * @memberof minimir.v1
             * @interface IHeartbeat
             * @augments minimir.v1.Heartbeat.$Properties
             * @deprecated Use minimir.v1.Heartbeat.$Properties instead.
             */

            /**
             * Shape of a Heartbeat.
             * @typedef {minimir.v1.Heartbeat.$Properties} minimir.v1.Heartbeat.$Shape
             */

            /**
             * Constructs a new Heartbeat.
             * @memberof minimir.v1
             * @classdesc Represents a Heartbeat.
             * @constructor
             * @param {minimir.v1.Heartbeat.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var Heartbeat = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * Heartbeat client_monotonic_ms.
             * @member {Long} client_monotonic_ms
             * @memberof minimir.v1.Heartbeat
             * @instance
             */
            Heartbeat.prototype.client_monotonic_ms = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new Heartbeat instance using the specified properties.
             * @function create
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {minimir.v1.Heartbeat.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.Heartbeat} Heartbeat instance
             * @type {{
             *   (properties: minimir.v1.Heartbeat.$Shape): minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape;
             *   (properties?: minimir.v1.Heartbeat.$Properties): minimir.v1.Heartbeat;
             * }}
             */
            Heartbeat.create = function(properties) {
                return new Heartbeat(properties);
            };

            /**
             * Encodes the specified Heartbeat message. Does not implicitly {@link minimir.v1.Heartbeat.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {minimir.v1.Heartbeat.$Properties} message Heartbeat message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Heartbeat.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.client_monotonic_ms != null && $Object.hasOwnProperty.call(message, "client_monotonic_ms") && (typeof message.client_monotonic_ms === "object" ? message.client_monotonic_ms.low || message.client_monotonic_ms.high : message.client_monotonic_ms !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.client_monotonic_ms);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified Heartbeat message, length delimited. Does not implicitly {@link minimir.v1.Heartbeat.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {minimir.v1.Heartbeat.$Properties} message Heartbeat message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Heartbeat.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a Heartbeat message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape} Heartbeat
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Heartbeat.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.Heartbeat();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.client_monotonic_ms = value;
                            else
                                delete message.client_monotonic_ms;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a Heartbeat message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.Heartbeat & minimir.v1.Heartbeat.$Shape} Heartbeat
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Heartbeat.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a Heartbeat message.
             * @function verify
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            Heartbeat.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.client_monotonic_ms != null && $Object.hasOwnProperty.call(message, "client_monotonic_ms"))
                    if (!$util.isInteger(message.client_monotonic_ms) && !(message.client_monotonic_ms && $util.isInteger(message.client_monotonic_ms.low) && $util.isInteger(message.client_monotonic_ms.high)))
                        return "client_monotonic_ms: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for Heartbeat
             * @function getTypeUrl
             * @memberof minimir.v1.Heartbeat
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            Heartbeat.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.Heartbeat";
            };

            return Heartbeat;
        })();

        v1.HeartbeatResult = (function() {

            /**
             * Properties of a HeartbeatResult.
             * @typedef {Object} minimir.v1.HeartbeatResult.$Properties
             * @property {Long|null} [server_unix_ms] HeartbeatResult server_unix_ms
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a HeartbeatResult.
             * @memberof minimir.v1
             * @interface IHeartbeatResult
             * @augments minimir.v1.HeartbeatResult.$Properties
             * @deprecated Use minimir.v1.HeartbeatResult.$Properties instead.
             */

            /**
             * Shape of a HeartbeatResult.
             * @typedef {minimir.v1.HeartbeatResult.$Properties} minimir.v1.HeartbeatResult.$Shape
             */

            /**
             * Constructs a new HeartbeatResult.
             * @memberof minimir.v1
             * @classdesc Represents a HeartbeatResult.
             * @constructor
             * @param {minimir.v1.HeartbeatResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var HeartbeatResult = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * HeartbeatResult server_unix_ms.
             * @member {Long} server_unix_ms
             * @memberof minimir.v1.HeartbeatResult
             * @instance
             */
            HeartbeatResult.prototype.server_unix_ms = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new HeartbeatResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {minimir.v1.HeartbeatResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.HeartbeatResult} HeartbeatResult instance
             * @type {{
             *   (properties: minimir.v1.HeartbeatResult.$Shape): minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape;
             *   (properties?: minimir.v1.HeartbeatResult.$Properties): minimir.v1.HeartbeatResult;
             * }}
             */
            HeartbeatResult.create = function(properties) {
                return new HeartbeatResult(properties);
            };

            /**
             * Encodes the specified HeartbeatResult message. Does not implicitly {@link minimir.v1.HeartbeatResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {minimir.v1.HeartbeatResult.$Properties} message HeartbeatResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HeartbeatResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.server_unix_ms != null && $Object.hasOwnProperty.call(message, "server_unix_ms") && (typeof message.server_unix_ms === "object" ? message.server_unix_ms.low || message.server_unix_ms.high : message.server_unix_ms !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.server_unix_ms);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified HeartbeatResult message, length delimited. Does not implicitly {@link minimir.v1.HeartbeatResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {minimir.v1.HeartbeatResult.$Properties} message HeartbeatResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HeartbeatResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a HeartbeatResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape} HeartbeatResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HeartbeatResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.HeartbeatResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.server_unix_ms = value;
                            else
                                delete message.server_unix_ms;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a HeartbeatResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.HeartbeatResult & minimir.v1.HeartbeatResult.$Shape} HeartbeatResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HeartbeatResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a HeartbeatResult message.
             * @function verify
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            HeartbeatResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.server_unix_ms != null && $Object.hasOwnProperty.call(message, "server_unix_ms"))
                    if (!$util.isInteger(message.server_unix_ms) && !(message.server_unix_ms && $util.isInteger(message.server_unix_ms.low) && $util.isInteger(message.server_unix_ms.high)))
                        return "server_unix_ms: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for HeartbeatResult
             * @function getTypeUrl
             * @memberof minimir.v1.HeartbeatResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            HeartbeatResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.HeartbeatResult";
            };

            return HeartbeatResult;
        })();

        v1.Batch = (function() {

            /**
             * Properties of a Batch.
             * @typedef {Object} minimir.v1.Batch.$Properties
             * @property {Array.<Uint8Array>|null} [application_bodies] Batch application_bodies
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a Batch.
             * @memberof minimir.v1
             * @interface IBatch
             * @augments minimir.v1.Batch.$Properties
             * @deprecated Use minimir.v1.Batch.$Properties instead.
             */

            /**
             * Shape of a Batch.
             * @typedef {minimir.v1.Batch.$Properties} minimir.v1.Batch.$Shape
             */

            /**
             * Constructs a new Batch.
             * @memberof minimir.v1
             * @classdesc Represents a Batch.
             * @constructor
             * @param {minimir.v1.Batch.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var Batch = function (properties) {
                this.application_bodies = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * Batch application_bodies.
             * @member {Array.<Uint8Array>} application_bodies
             * @memberof minimir.v1.Batch
             * @instance
             */
            Batch.prototype.application_bodies = $util.emptyArray;

            /**
             * Creates a new Batch instance using the specified properties.
             * @function create
             * @memberof minimir.v1.Batch
             * @static
             * @param {minimir.v1.Batch.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.Batch} Batch instance
             * @type {{
             *   (properties: minimir.v1.Batch.$Shape): minimir.v1.Batch & minimir.v1.Batch.$Shape;
             *   (properties?: minimir.v1.Batch.$Properties): minimir.v1.Batch;
             * }}
             */
            Batch.create = function(properties) {
                return new Batch(properties);
            };

            /**
             * Encodes the specified Batch message. Does not implicitly {@link minimir.v1.Batch.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.Batch
             * @static
             * @param {minimir.v1.Batch.$Properties} message Batch message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Batch.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.application_bodies != null && message.application_bodies.length)
                    for (var i = 0; i < message.application_bodies.length; ++i)
                        writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.application_bodies[i]);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified Batch message, length delimited. Does not implicitly {@link minimir.v1.Batch.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.Batch
             * @static
             * @param {minimir.v1.Batch.$Properties} message Batch message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            Batch.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a Batch message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.Batch
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.Batch & minimir.v1.Batch.$Shape} Batch
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Batch.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.Batch();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if (!(message.application_bodies && message.application_bodies.length))
                                message.application_bodies = [];
                            message.application_bodies.push(reader.bytes());
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a Batch message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.Batch
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.Batch & minimir.v1.Batch.$Shape} Batch
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            Batch.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a Batch message.
             * @function verify
             * @memberof minimir.v1.Batch
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            Batch.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.application_bodies != null && $Object.hasOwnProperty.call(message, "application_bodies")) {
                    if (!$Array.isArray(message.application_bodies))
                        return "application_bodies: array expected";
                    for (var i = 0; i < message.application_bodies.length; ++i)
                        if (!(message.application_bodies[i] && typeof message.application_bodies[i].length === "number" || $util.isString(message.application_bodies[i])))
                            return "application_bodies: buffer[] expected";
                }
                return null;
            };

            /**
             * Gets the type url for Batch
             * @function getTypeUrl
             * @memberof minimir.v1.Batch
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            Batch.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.Batch";
            };

            return Batch;
        })();

        v1.ConfigVersion = (function() {

            /**
             * Properties of a ConfigVersion.
             * @typedef {Object} minimir.v1.ConfigVersion.$Properties
             * @property {Long|null} [rule_version] ConfigVersion rule_version
             * @property {string|null} [content_hash] ConfigVersion content_hash
             * @property {Long|null} [resource_version] ConfigVersion resource_version
             * @property {number|null} [protocol_version] ConfigVersion protocol_version
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a ConfigVersion.
             * @memberof minimir.v1
             * @interface IConfigVersion
             * @augments minimir.v1.ConfigVersion.$Properties
             * @deprecated Use minimir.v1.ConfigVersion.$Properties instead.
             */

            /**
             * Shape of a ConfigVersion.
             * @typedef {minimir.v1.ConfigVersion.$Properties} minimir.v1.ConfigVersion.$Shape
             */

            /**
             * Constructs a new ConfigVersion.
             * @memberof minimir.v1
             * @classdesc Represents a ConfigVersion.
             * @constructor
             * @param {minimir.v1.ConfigVersion.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var ConfigVersion = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * ConfigVersion rule_version.
             * @member {Long} rule_version
             * @memberof minimir.v1.ConfigVersion
             * @instance
             */
            ConfigVersion.prototype.rule_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * ConfigVersion content_hash.
             * @member {string} content_hash
             * @memberof minimir.v1.ConfigVersion
             * @instance
             */
            ConfigVersion.prototype.content_hash = "";

            /**
             * ConfigVersion resource_version.
             * @member {Long} resource_version
             * @memberof minimir.v1.ConfigVersion
             * @instance
             */
            ConfigVersion.prototype.resource_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * ConfigVersion protocol_version.
             * @member {number} protocol_version
             * @memberof minimir.v1.ConfigVersion
             * @instance
             */
            ConfigVersion.prototype.protocol_version = 0;

            /**
             * Creates a new ConfigVersion instance using the specified properties.
             * @function create
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {minimir.v1.ConfigVersion.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.ConfigVersion} ConfigVersion instance
             * @type {{
             *   (properties: minimir.v1.ConfigVersion.$Shape): minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape;
             *   (properties?: minimir.v1.ConfigVersion.$Properties): minimir.v1.ConfigVersion;
             * }}
             */
            ConfigVersion.create = function(properties) {
                return new ConfigVersion(properties);
            };

            /**
             * Encodes the specified ConfigVersion message. Does not implicitly {@link minimir.v1.ConfigVersion.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {minimir.v1.ConfigVersion.$Properties} message ConfigVersion message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ConfigVersion.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version") && (typeof message.rule_version === "object" ? message.rule_version.low || message.rule_version.high : message.rule_version !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.rule_version);
                if (message.content_hash != null && $Object.hasOwnProperty.call(message, "content_hash") && message.content_hash !== "")
                    writer.uint32(/* id 2, wireType 2 =*/18).string(message.content_hash);
                if (message.resource_version != null && $Object.hasOwnProperty.call(message, "resource_version") && (typeof message.resource_version === "object" ? message.resource_version.low || message.resource_version.high : message.resource_version !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.resource_version);
                if (message.protocol_version != null && $Object.hasOwnProperty.call(message, "protocol_version") && message.protocol_version !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.protocol_version);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified ConfigVersion message, length delimited. Does not implicitly {@link minimir.v1.ConfigVersion.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {minimir.v1.ConfigVersion.$Properties} message ConfigVersion message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ConfigVersion.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a ConfigVersion message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape} ConfigVersion
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ConfigVersion.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.ConfigVersion();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.rule_version = value;
                            else
                                delete message.rule_version;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.content_hash = value;
                            else
                                delete message.content_hash;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.resource_version = value;
                            else
                                delete message.resource_version;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.protocol_version = value;
                            else
                                delete message.protocol_version;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a ConfigVersion message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.ConfigVersion & minimir.v1.ConfigVersion.$Shape} ConfigVersion
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ConfigVersion.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a ConfigVersion message.
             * @function verify
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            ConfigVersion.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version"))
                    if (!$util.isInteger(message.rule_version) && !(message.rule_version && $util.isInteger(message.rule_version.low) && $util.isInteger(message.rule_version.high)))
                        return "rule_version: integer|Long expected";
                if (message.content_hash != null && $Object.hasOwnProperty.call(message, "content_hash"))
                    if (!$util.isString(message.content_hash))
                        return "content_hash: string expected";
                if (message.resource_version != null && $Object.hasOwnProperty.call(message, "resource_version"))
                    if (!$util.isInteger(message.resource_version) && !(message.resource_version && $util.isInteger(message.resource_version.low) && $util.isInteger(message.resource_version.high)))
                        return "resource_version: integer|Long expected";
                if (message.protocol_version != null && $Object.hasOwnProperty.call(message, "protocol_version"))
                    if (!$util.isInteger(message.protocol_version))
                        return "protocol_version: integer expected";
                return null;
            };

            /**
             * Gets the type url for ConfigVersion
             * @function getTypeUrl
             * @memberof minimir.v1.ConfigVersion
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            ConfigVersion.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.ConfigVersion";
            };

            return ConfigVersion;
        })();

        v1.LocalLoginRequest = (function() {

            /**
             * Properties of a LocalLoginRequest.
             * @typedef {Object} minimir.v1.LocalLoginRequest.$Properties
             * @property {string|null} [account] LocalLoginRequest account
             * @property {string|null} [password] LocalLoginRequest password
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a LocalLoginRequest.
             * @memberof minimir.v1
             * @interface ILocalLoginRequest
             * @augments minimir.v1.LocalLoginRequest.$Properties
             * @deprecated Use minimir.v1.LocalLoginRequest.$Properties instead.
             */

            /**
             * Shape of a LocalLoginRequest.
             * @typedef {minimir.v1.LocalLoginRequest.$Properties} minimir.v1.LocalLoginRequest.$Shape
             */

            /**
             * Constructs a new LocalLoginRequest.
             * @memberof minimir.v1
             * @classdesc Represents a LocalLoginRequest.
             * @constructor
             * @param {minimir.v1.LocalLoginRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var LocalLoginRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * LocalLoginRequest account.
             * @member {string} account
             * @memberof minimir.v1.LocalLoginRequest
             * @instance
             */
            LocalLoginRequest.prototype.account = "";

            /**
             * LocalLoginRequest password.
             * @member {string} password
             * @memberof minimir.v1.LocalLoginRequest
             * @instance
             */
            LocalLoginRequest.prototype.password = "";

            /**
             * Creates a new LocalLoginRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {minimir.v1.LocalLoginRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.LocalLoginRequest} LocalLoginRequest instance
             * @type {{
             *   (properties: minimir.v1.LocalLoginRequest.$Shape): minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape;
             *   (properties?: minimir.v1.LocalLoginRequest.$Properties): minimir.v1.LocalLoginRequest;
             * }}
             */
            LocalLoginRequest.create = function(properties) {
                return new LocalLoginRequest(properties);
            };

            /**
             * Encodes the specified LocalLoginRequest message. Does not implicitly {@link minimir.v1.LocalLoginRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {minimir.v1.LocalLoginRequest.$Properties} message LocalLoginRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LocalLoginRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.account != null && $Object.hasOwnProperty.call(message, "account") && message.account !== "")
                    writer.uint32(/* id 1, wireType 2 =*/10).string(message.account);
                if (message.password != null && $Object.hasOwnProperty.call(message, "password") && message.password !== "")
                    writer.uint32(/* id 2, wireType 2 =*/18).string(message.password);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified LocalLoginRequest message, length delimited. Does not implicitly {@link minimir.v1.LocalLoginRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {minimir.v1.LocalLoginRequest.$Properties} message LocalLoginRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LocalLoginRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a LocalLoginRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape} LocalLoginRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LocalLoginRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.LocalLoginRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.account = value;
                            else
                                delete message.account;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.password = value;
                            else
                                delete message.password;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a LocalLoginRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.LocalLoginRequest & minimir.v1.LocalLoginRequest.$Shape} LocalLoginRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LocalLoginRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a LocalLoginRequest message.
             * @function verify
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            LocalLoginRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.account != null && $Object.hasOwnProperty.call(message, "account"))
                    if (!$util.isString(message.account))
                        return "account: string expected";
                if (message.password != null && $Object.hasOwnProperty.call(message, "password"))
                    if (!$util.isString(message.password))
                        return "password: string expected";
                return null;
            };

            /**
             * Gets the type url for LocalLoginRequest
             * @function getTypeUrl
             * @memberof minimir.v1.LocalLoginRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            LocalLoginRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.LocalLoginRequest";
            };

            return LocalLoginRequest;
        })();

        v1.LoginResult = (function() {

            /**
             * Properties of a LoginResult.
             * @typedef {Object} minimir.v1.LoginResult.$Properties
             * @property {Uint8Array|null} [account_id] LoginResult account_id
             * @property {string|null} [account_token] LoginResult account_token
             * @property {Long|null} [expires_at_unix] LoginResult expires_at_unix
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a LoginResult.
             * @memberof minimir.v1
             * @interface ILoginResult
             * @augments minimir.v1.LoginResult.$Properties
             * @deprecated Use minimir.v1.LoginResult.$Properties instead.
             */

            /**
             * Shape of a LoginResult.
             * @typedef {minimir.v1.LoginResult.$Properties} minimir.v1.LoginResult.$Shape
             */

            /**
             * Constructs a new LoginResult.
             * @memberof minimir.v1
             * @classdesc Represents a LoginResult.
             * @constructor
             * @param {minimir.v1.LoginResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var LoginResult = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * LoginResult account_id.
             * @member {Uint8Array} account_id
             * @memberof minimir.v1.LoginResult
             * @instance
             */
            LoginResult.prototype.account_id = $util.newBuffer([]);

            /**
             * LoginResult account_token.
             * @member {string} account_token
             * @memberof minimir.v1.LoginResult
             * @instance
             */
            LoginResult.prototype.account_token = "";

            /**
             * LoginResult expires_at_unix.
             * @member {Long} expires_at_unix
             * @memberof minimir.v1.LoginResult
             * @instance
             */
            LoginResult.prototype.expires_at_unix = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new LoginResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {minimir.v1.LoginResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.LoginResult} LoginResult instance
             * @type {{
             *   (properties: minimir.v1.LoginResult.$Shape): minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape;
             *   (properties?: minimir.v1.LoginResult.$Properties): minimir.v1.LoginResult;
             * }}
             */
            LoginResult.create = function(properties) {
                return new LoginResult(properties);
            };

            /**
             * Encodes the specified LoginResult message. Does not implicitly {@link minimir.v1.LoginResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {minimir.v1.LoginResult.$Properties} message LoginResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LoginResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.account_id != null && $Object.hasOwnProperty.call(message, "account_id") && message.account_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.account_id);
                if (message.account_token != null && $Object.hasOwnProperty.call(message, "account_token") && message.account_token !== "")
                    writer.uint32(/* id 2, wireType 2 =*/18).string(message.account_token);
                if (message.expires_at_unix != null && $Object.hasOwnProperty.call(message, "expires_at_unix") && (typeof message.expires_at_unix === "object" ? message.expires_at_unix.low || message.expires_at_unix.high : message.expires_at_unix !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.expires_at_unix);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified LoginResult message, length delimited. Does not implicitly {@link minimir.v1.LoginResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {minimir.v1.LoginResult.$Properties} message LoginResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LoginResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a LoginResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape} LoginResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LoginResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.LoginResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.account_id = value;
                            else
                                delete message.account_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.account_token = value;
                            else
                                delete message.account_token;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.expires_at_unix = value;
                            else
                                delete message.expires_at_unix;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a LoginResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.LoginResult & minimir.v1.LoginResult.$Shape} LoginResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LoginResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a LoginResult message.
             * @function verify
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            LoginResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.account_id != null && $Object.hasOwnProperty.call(message, "account_id"))
                    if (!(message.account_id && typeof message.account_id.length === "number" || $util.isString(message.account_id)))
                        return "account_id: buffer expected";
                if (message.account_token != null && $Object.hasOwnProperty.call(message, "account_token"))
                    if (!$util.isString(message.account_token))
                        return "account_token: string expected";
                if (message.expires_at_unix != null && $Object.hasOwnProperty.call(message, "expires_at_unix"))
                    if (!$util.isInteger(message.expires_at_unix) && !(message.expires_at_unix && $util.isInteger(message.expires_at_unix.low) && $util.isInteger(message.expires_at_unix.high)))
                        return "expires_at_unix: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for LoginResult
             * @function getTypeUrl
             * @memberof minimir.v1.LoginResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            LoginResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.LoginResult";
            };

            return LoginResult;
        })();

        v1.RoleDirectoryRequest = (function() {

            /**
             * Properties of a RoleDirectoryRequest.
             * @typedef {Object} minimir.v1.RoleDirectoryRequest.$Properties
             * @property {Long|null} [origin_server_id] RoleDirectoryRequest origin_server_id
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a RoleDirectoryRequest.
             * @memberof minimir.v1
             * @interface IRoleDirectoryRequest
             * @augments minimir.v1.RoleDirectoryRequest.$Properties
             * @deprecated Use minimir.v1.RoleDirectoryRequest.$Properties instead.
             */

            /**
             * Shape of a RoleDirectoryRequest.
             * @typedef {minimir.v1.RoleDirectoryRequest.$Properties} minimir.v1.RoleDirectoryRequest.$Shape
             */

            /**
             * Constructs a new RoleDirectoryRequest.
             * @memberof minimir.v1
             * @classdesc Represents a RoleDirectoryRequest.
             * @constructor
             * @param {minimir.v1.RoleDirectoryRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var RoleDirectoryRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * RoleDirectoryRequest origin_server_id.
             * @member {Long} origin_server_id
             * @memberof minimir.v1.RoleDirectoryRequest
             * @instance
             */
            RoleDirectoryRequest.prototype.origin_server_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new RoleDirectoryRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {minimir.v1.RoleDirectoryRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.RoleDirectoryRequest} RoleDirectoryRequest instance
             * @type {{
             *   (properties: minimir.v1.RoleDirectoryRequest.$Shape): minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape;
             *   (properties?: minimir.v1.RoleDirectoryRequest.$Properties): minimir.v1.RoleDirectoryRequest;
             * }}
             */
            RoleDirectoryRequest.create = function(properties) {
                return new RoleDirectoryRequest(properties);
            };

            /**
             * Encodes the specified RoleDirectoryRequest message. Does not implicitly {@link minimir.v1.RoleDirectoryRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {minimir.v1.RoleDirectoryRequest.$Properties} message RoleDirectoryRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleDirectoryRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id") && (typeof message.origin_server_id === "object" ? message.origin_server_id.low || message.origin_server_id.high : message.origin_server_id !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.origin_server_id);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified RoleDirectoryRequest message, length delimited. Does not implicitly {@link minimir.v1.RoleDirectoryRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {minimir.v1.RoleDirectoryRequest.$Properties} message RoleDirectoryRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleDirectoryRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a RoleDirectoryRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape} RoleDirectoryRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleDirectoryRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.RoleDirectoryRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.origin_server_id = value;
                            else
                                delete message.origin_server_id;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a RoleDirectoryRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleDirectoryRequest & minimir.v1.RoleDirectoryRequest.$Shape} RoleDirectoryRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleDirectoryRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a RoleDirectoryRequest message.
             * @function verify
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            RoleDirectoryRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id"))
                    if (!$util.isInteger(message.origin_server_id) && !(message.origin_server_id && $util.isInteger(message.origin_server_id.low) && $util.isInteger(message.origin_server_id.high)))
                        return "origin_server_id: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for RoleDirectoryRequest
             * @function getTypeUrl
             * @memberof minimir.v1.RoleDirectoryRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            RoleDirectoryRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.RoleDirectoryRequest";
            };

            return RoleDirectoryRequest;
        })();

        v1.RoleSummary = (function() {

            /**
             * Properties of a RoleSummary.
             * @typedef {Object} minimir.v1.RoleSummary.$Properties
             * @property {minimir.v1.RoleKey.$Properties|null} [key] RoleSummary key
             * @property {string|null} [name] RoleSummary name
             * @property {number|null} [job] RoleSummary job
             * @property {number|null} [gender] RoleSummary gender
             * @property {number|null} [level] RoleSummary level
             * @property {number|null} [slot] RoleSummary slot
             * @property {Long|null} [appearance_version] RoleSummary appearance_version
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a RoleSummary.
             * @memberof minimir.v1
             * @interface IRoleSummary
             * @augments minimir.v1.RoleSummary.$Properties
             * @deprecated Use minimir.v1.RoleSummary.$Properties instead.
             */

            /**
             * Shape of a RoleSummary.
             * @typedef {minimir.v1.RoleSummary.$Properties} minimir.v1.RoleSummary.$Shape
             */

            /**
             * Constructs a new RoleSummary.
             * @memberof minimir.v1
             * @classdesc Represents a RoleSummary.
             * @constructor
             * @param {minimir.v1.RoleSummary.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var RoleSummary = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * RoleSummary key.
             * @member {minimir.v1.RoleKey.$Properties|null|undefined} key
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.key = null;

            /**
             * RoleSummary name.
             * @member {string} name
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.name = "";

            /**
             * RoleSummary job.
             * @member {number} job
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.job = 0;

            /**
             * RoleSummary gender.
             * @member {number} gender
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.gender = 0;

            /**
             * RoleSummary level.
             * @member {number} level
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.level = 0;

            /**
             * RoleSummary slot.
             * @member {number} slot
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.slot = 0;

            /**
             * RoleSummary appearance_version.
             * @member {Long} appearance_version
             * @memberof minimir.v1.RoleSummary
             * @instance
             */
            RoleSummary.prototype.appearance_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new RoleSummary instance using the specified properties.
             * @function create
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {minimir.v1.RoleSummary.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.RoleSummary} RoleSummary instance
             * @type {{
             *   (properties: minimir.v1.RoleSummary.$Shape): minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape;
             *   (properties?: minimir.v1.RoleSummary.$Properties): minimir.v1.RoleSummary;
             * }}
             */
            RoleSummary.create = function(properties) {
                return new RoleSummary(properties);
            };

            /**
             * Encodes the specified RoleSummary message. Does not implicitly {@link minimir.v1.RoleSummary.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {minimir.v1.RoleSummary.$Properties} message RoleSummary message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleSummary.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.key != null && $Object.hasOwnProperty.call(message, "key"))
                    $root.minimir.v1.RoleKey.encode(message.key, writer.uint32(/* id 1, wireType 2 =*/10).fork(), _depth + 1).ldelim();
                if (message.name != null && $Object.hasOwnProperty.call(message, "name") && message.name !== "")
                    writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
                if (message.job != null && $Object.hasOwnProperty.call(message, "job") && message.job !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.job);
                if (message.gender != null && $Object.hasOwnProperty.call(message, "gender") && message.gender !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.gender);
                if (message.level != null && $Object.hasOwnProperty.call(message, "level") && message.level !== 0)
                    writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.level);
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot") && message.slot !== 0)
                    writer.uint32(/* id 6, wireType 0 =*/48).uint32(message.slot);
                if (message.appearance_version != null && $Object.hasOwnProperty.call(message, "appearance_version") && (typeof message.appearance_version === "object" ? message.appearance_version.low || message.appearance_version.high : message.appearance_version !== 0))
                    writer.uint32(/* id 7, wireType 0 =*/56).uint64(message.appearance_version);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified RoleSummary message, length delimited. Does not implicitly {@link minimir.v1.RoleSummary.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {minimir.v1.RoleSummary.$Properties} message RoleSummary message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleSummary.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a RoleSummary message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape} RoleSummary
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleSummary.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.RoleSummary();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            message.key = $root.minimir.v1.RoleKey.decode(reader, reader.size(), $undefined, _depth + 1, message.key);
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.name = value;
                            else
                                delete message.name;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.job = value;
                            else
                                delete message.job;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.gender = value;
                            else
                                delete message.gender;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.level = value;
                            else
                                delete message.level;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.slot = value;
                            else
                                delete message.slot;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.appearance_version = value;
                            else
                                delete message.appearance_version;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a RoleSummary message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleSummary & minimir.v1.RoleSummary.$Shape} RoleSummary
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleSummary.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a RoleSummary message.
             * @function verify
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            RoleSummary.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.key != null && $Object.hasOwnProperty.call(message, "key")) {
                    var error = $root.minimir.v1.RoleKey.verify(message.key, _depth + 1);
                    if (error)
                        return "key." + error;
                }
                if (message.name != null && $Object.hasOwnProperty.call(message, "name"))
                    if (!$util.isString(message.name))
                        return "name: string expected";
                if (message.job != null && $Object.hasOwnProperty.call(message, "job"))
                    if (!$util.isInteger(message.job))
                        return "job: integer expected";
                if (message.gender != null && $Object.hasOwnProperty.call(message, "gender"))
                    if (!$util.isInteger(message.gender))
                        return "gender: integer expected";
                if (message.level != null && $Object.hasOwnProperty.call(message, "level"))
                    if (!$util.isInteger(message.level))
                        return "level: integer expected";
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot"))
                    if (!$util.isInteger(message.slot))
                        return "slot: integer expected";
                if (message.appearance_version != null && $Object.hasOwnProperty.call(message, "appearance_version"))
                    if (!$util.isInteger(message.appearance_version) && !(message.appearance_version && $util.isInteger(message.appearance_version.low) && $util.isInteger(message.appearance_version.high)))
                        return "appearance_version: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for RoleSummary
             * @function getTypeUrl
             * @memberof minimir.v1.RoleSummary
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            RoleSummary.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.RoleSummary";
            };

            return RoleSummary;
        })();

        v1.RoleDirectoryResult = (function() {

            /**
             * Properties of a RoleDirectoryResult.
             * @typedef {Object} minimir.v1.RoleDirectoryResult.$Properties
             * @property {Array.<minimir.v1.RoleSummary.$Properties>|null} [roles] RoleDirectoryResult roles
             * @property {Long|null} [route_version] RoleDirectoryResult route_version
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a RoleDirectoryResult.
             * @memberof minimir.v1
             * @interface IRoleDirectoryResult
             * @augments minimir.v1.RoleDirectoryResult.$Properties
             * @deprecated Use minimir.v1.RoleDirectoryResult.$Properties instead.
             */

            /**
             * Shape of a RoleDirectoryResult.
             * @typedef {minimir.v1.RoleDirectoryResult.$Properties} minimir.v1.RoleDirectoryResult.$Shape
             */

            /**
             * Constructs a new RoleDirectoryResult.
             * @memberof minimir.v1
             * @classdesc Represents a RoleDirectoryResult.
             * @constructor
             * @param {minimir.v1.RoleDirectoryResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var RoleDirectoryResult = function (properties) {
                this.roles = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * RoleDirectoryResult roles.
             * @member {Array.<minimir.v1.RoleSummary.$Properties>} roles
             * @memberof minimir.v1.RoleDirectoryResult
             * @instance
             */
            RoleDirectoryResult.prototype.roles = $util.emptyArray;

            /**
             * RoleDirectoryResult route_version.
             * @member {Long} route_version
             * @memberof minimir.v1.RoleDirectoryResult
             * @instance
             */
            RoleDirectoryResult.prototype.route_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new RoleDirectoryResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {minimir.v1.RoleDirectoryResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.RoleDirectoryResult} RoleDirectoryResult instance
             * @type {{
             *   (properties: minimir.v1.RoleDirectoryResult.$Shape): minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape;
             *   (properties?: minimir.v1.RoleDirectoryResult.$Properties): minimir.v1.RoleDirectoryResult;
             * }}
             */
            RoleDirectoryResult.create = function(properties) {
                return new RoleDirectoryResult(properties);
            };

            /**
             * Encodes the specified RoleDirectoryResult message. Does not implicitly {@link minimir.v1.RoleDirectoryResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {minimir.v1.RoleDirectoryResult.$Properties} message RoleDirectoryResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleDirectoryResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.roles != null && message.roles.length)
                    for (var i = 0; i < message.roles.length; ++i)
                        $root.minimir.v1.RoleSummary.encode(message.roles[i], writer.uint32(/* id 1, wireType 2 =*/10).fork(), _depth + 1).ldelim();
                if (message.route_version != null && $Object.hasOwnProperty.call(message, "route_version") && (typeof message.route_version === "object" ? message.route_version.low || message.route_version.high : message.route_version !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.route_version);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified RoleDirectoryResult message, length delimited. Does not implicitly {@link minimir.v1.RoleDirectoryResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {minimir.v1.RoleDirectoryResult.$Properties} message RoleDirectoryResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RoleDirectoryResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a RoleDirectoryResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape} RoleDirectoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleDirectoryResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.RoleDirectoryResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if (!(message.roles && message.roles.length))
                                message.roles = [];
                            message.roles.push($root.minimir.v1.RoleSummary.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.route_version = value;
                            else
                                delete message.route_version;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a RoleDirectoryResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.RoleDirectoryResult & minimir.v1.RoleDirectoryResult.$Shape} RoleDirectoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RoleDirectoryResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a RoleDirectoryResult message.
             * @function verify
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            RoleDirectoryResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.roles != null && $Object.hasOwnProperty.call(message, "roles")) {
                    if (!$Array.isArray(message.roles))
                        return "roles: array expected";
                    for (var i = 0; i < message.roles.length; ++i) {
                        var error = $root.minimir.v1.RoleSummary.verify(message.roles[i], _depth + 1);
                        if (error)
                            return "roles." + error;
                    }
                }
                if (message.route_version != null && $Object.hasOwnProperty.call(message, "route_version"))
                    if (!$util.isInteger(message.route_version) && !(message.route_version && $util.isInteger(message.route_version.low) && $util.isInteger(message.route_version.high)))
                        return "route_version: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for RoleDirectoryResult
             * @function getTypeUrl
             * @memberof minimir.v1.RoleDirectoryResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            RoleDirectoryResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.RoleDirectoryResult";
            };

            return RoleDirectoryResult;
        })();

        v1.CreateRoleRequest = (function() {

            /**
             * Properties of a CreateRoleRequest.
             * @typedef {Object} minimir.v1.CreateRoleRequest.$Properties
             * @property {Long|null} [origin_server_id] CreateRoleRequest origin_server_id
             * @property {number|null} [slot] CreateRoleRequest slot
             * @property {number|null} [job] CreateRoleRequest job
             * @property {string|null} [name] CreateRoleRequest name
             * @property {Uint8Array|null} [op_id] CreateRoleRequest op_id
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a CreateRoleRequest.
             * @memberof minimir.v1
             * @interface ICreateRoleRequest
             * @augments minimir.v1.CreateRoleRequest.$Properties
             * @deprecated Use minimir.v1.CreateRoleRequest.$Properties instead.
             */

            /**
             * Shape of a CreateRoleRequest.
             * @typedef {minimir.v1.CreateRoleRequest.$Properties} minimir.v1.CreateRoleRequest.$Shape
             */

            /**
             * Constructs a new CreateRoleRequest.
             * @memberof minimir.v1
             * @classdesc Represents a CreateRoleRequest.
             * @constructor
             * @param {minimir.v1.CreateRoleRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var CreateRoleRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * CreateRoleRequest origin_server_id.
             * @member {Long} origin_server_id
             * @memberof minimir.v1.CreateRoleRequest
             * @instance
             */
            CreateRoleRequest.prototype.origin_server_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CreateRoleRequest slot.
             * @member {number} slot
             * @memberof minimir.v1.CreateRoleRequest
             * @instance
             */
            CreateRoleRequest.prototype.slot = 0;

            /**
             * CreateRoleRequest job.
             * @member {number} job
             * @memberof minimir.v1.CreateRoleRequest
             * @instance
             */
            CreateRoleRequest.prototype.job = 0;

            /**
             * CreateRoleRequest name.
             * @member {string} name
             * @memberof minimir.v1.CreateRoleRequest
             * @instance
             */
            CreateRoleRequest.prototype.name = "";

            /**
             * CreateRoleRequest op_id.
             * @member {Uint8Array} op_id
             * @memberof minimir.v1.CreateRoleRequest
             * @instance
             */
            CreateRoleRequest.prototype.op_id = $util.newBuffer([]);

            /**
             * Creates a new CreateRoleRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {minimir.v1.CreateRoleRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.CreateRoleRequest} CreateRoleRequest instance
             * @type {{
             *   (properties: minimir.v1.CreateRoleRequest.$Shape): minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape;
             *   (properties?: minimir.v1.CreateRoleRequest.$Properties): minimir.v1.CreateRoleRequest;
             * }}
             */
            CreateRoleRequest.create = function(properties) {
                return new CreateRoleRequest(properties);
            };

            /**
             * Encodes the specified CreateRoleRequest message. Does not implicitly {@link minimir.v1.CreateRoleRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {minimir.v1.CreateRoleRequest.$Properties} message CreateRoleRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CreateRoleRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id") && (typeof message.origin_server_id === "object" ? message.origin_server_id.low || message.origin_server_id.high : message.origin_server_id !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.origin_server_id);
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot") && message.slot !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.slot);
                if (message.job != null && $Object.hasOwnProperty.call(message, "job") && message.job !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.job);
                if (message.name != null && $Object.hasOwnProperty.call(message, "name") && message.name !== "")
                    writer.uint32(/* id 4, wireType 2 =*/34).string(message.name);
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id") && message.op_id.length)
                    writer.uint32(/* id 5, wireType 2 =*/42).bytes(message.op_id);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified CreateRoleRequest message, length delimited. Does not implicitly {@link minimir.v1.CreateRoleRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {minimir.v1.CreateRoleRequest.$Properties} message CreateRoleRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CreateRoleRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a CreateRoleRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape} CreateRoleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CreateRoleRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.CreateRoleRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.origin_server_id = value;
                            else
                                delete message.origin_server_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.slot = value;
                            else
                                delete message.slot;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.job = value;
                            else
                                delete message.job;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.name = value;
                            else
                                delete message.name;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.op_id = value;
                            else
                                delete message.op_id;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a CreateRoleRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.CreateRoleRequest & minimir.v1.CreateRoleRequest.$Shape} CreateRoleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CreateRoleRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a CreateRoleRequest message.
             * @function verify
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            CreateRoleRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.origin_server_id != null && $Object.hasOwnProperty.call(message, "origin_server_id"))
                    if (!$util.isInteger(message.origin_server_id) && !(message.origin_server_id && $util.isInteger(message.origin_server_id.low) && $util.isInteger(message.origin_server_id.high)))
                        return "origin_server_id: integer|Long expected";
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot"))
                    if (!$util.isInteger(message.slot))
                        return "slot: integer expected";
                if (message.job != null && $Object.hasOwnProperty.call(message, "job"))
                    if (!$util.isInteger(message.job))
                        return "job: integer expected";
                if (message.name != null && $Object.hasOwnProperty.call(message, "name"))
                    if (!$util.isString(message.name))
                        return "name: string expected";
                if (message.op_id != null && $Object.hasOwnProperty.call(message, "op_id"))
                    if (!(message.op_id && typeof message.op_id.length === "number" || $util.isString(message.op_id)))
                        return "op_id: buffer expected";
                return null;
            };

            /**
             * Gets the type url for CreateRoleRequest
             * @function getTypeUrl
             * @memberof minimir.v1.CreateRoleRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            CreateRoleRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.CreateRoleRequest";
            };

            return CreateRoleRequest;
        })();

        v1.EntryTicketRequest = (function() {

            /**
             * Properties of an EntryTicketRequest.
             * @typedef {Object} minimir.v1.EntryTicketRequest.$Properties
             * @property {minimir.v1.RoleKey.$Properties|null} [role] EntryTicketRequest role
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EntryTicketRequest.
             * @memberof minimir.v1
             * @interface IEntryTicketRequest
             * @augments minimir.v1.EntryTicketRequest.$Properties
             * @deprecated Use minimir.v1.EntryTicketRequest.$Properties instead.
             */

            /**
             * Shape of an EntryTicketRequest.
             * @typedef {minimir.v1.EntryTicketRequest.$Properties} minimir.v1.EntryTicketRequest.$Shape
             */

            /**
             * Constructs a new EntryTicketRequest.
             * @memberof minimir.v1
             * @classdesc Represents an EntryTicketRequest.
             * @constructor
             * @param {minimir.v1.EntryTicketRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EntryTicketRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EntryTicketRequest role.
             * @member {minimir.v1.RoleKey.$Properties|null|undefined} role
             * @memberof minimir.v1.EntryTicketRequest
             * @instance
             */
            EntryTicketRequest.prototype.role = null;

            /**
             * Creates a new EntryTicketRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {minimir.v1.EntryTicketRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EntryTicketRequest} EntryTicketRequest instance
             * @type {{
             *   (properties: minimir.v1.EntryTicketRequest.$Shape): minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape;
             *   (properties?: minimir.v1.EntryTicketRequest.$Properties): minimir.v1.EntryTicketRequest;
             * }}
             */
            EntryTicketRequest.create = function(properties) {
                return new EntryTicketRequest(properties);
            };

            /**
             * Encodes the specified EntryTicketRequest message. Does not implicitly {@link minimir.v1.EntryTicketRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {minimir.v1.EntryTicketRequest.$Properties} message EntryTicketRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntryTicketRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.role != null && $Object.hasOwnProperty.call(message, "role"))
                    $root.minimir.v1.RoleKey.encode(message.role, writer.uint32(/* id 1, wireType 2 =*/10).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EntryTicketRequest message, length delimited. Does not implicitly {@link minimir.v1.EntryTicketRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {minimir.v1.EntryTicketRequest.$Properties} message EntryTicketRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntryTicketRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EntryTicketRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape} EntryTicketRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntryTicketRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EntryTicketRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            message.role = $root.minimir.v1.RoleKey.decode(reader, reader.size(), $undefined, _depth + 1, message.role);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EntryTicketRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EntryTicketRequest & minimir.v1.EntryTicketRequest.$Shape} EntryTicketRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntryTicketRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EntryTicketRequest message.
             * @function verify
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EntryTicketRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.role != null && $Object.hasOwnProperty.call(message, "role")) {
                    var error = $root.minimir.v1.RoleKey.verify(message.role, _depth + 1);
                    if (error)
                        return "role." + error;
                }
                return null;
            };

            /**
             * Gets the type url for EntryTicketRequest
             * @function getTypeUrl
             * @memberof minimir.v1.EntryTicketRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EntryTicketRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EntryTicketRequest";
            };

            return EntryTicketRequest;
        })();

        v1.EntryTicketResult = (function() {

            /**
             * Properties of an EntryTicketResult.
             * @typedef {Object} minimir.v1.EntryTicketResult.$Properties
             * @property {string|null} [ticket] EntryTicketResult ticket
             * @property {string|null} [game_wss_url] EntryTicketResult game_wss_url
             * @property {Long|null} [run_server_id] EntryTicketResult run_server_id
             * @property {Long|null} [route_version] EntryTicketResult route_version
             * @property {Long|null} [expires_at_unix] EntryTicketResult expires_at_unix
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EntryTicketResult.
             * @memberof minimir.v1
             * @interface IEntryTicketResult
             * @augments minimir.v1.EntryTicketResult.$Properties
             * @deprecated Use minimir.v1.EntryTicketResult.$Properties instead.
             */

            /**
             * Shape of an EntryTicketResult.
             * @typedef {minimir.v1.EntryTicketResult.$Properties} minimir.v1.EntryTicketResult.$Shape
             */

            /**
             * Constructs a new EntryTicketResult.
             * @memberof minimir.v1
             * @classdesc Represents an EntryTicketResult.
             * @constructor
             * @param {minimir.v1.EntryTicketResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EntryTicketResult = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EntryTicketResult ticket.
             * @member {string} ticket
             * @memberof minimir.v1.EntryTicketResult
             * @instance
             */
            EntryTicketResult.prototype.ticket = "";

            /**
             * EntryTicketResult game_wss_url.
             * @member {string} game_wss_url
             * @memberof minimir.v1.EntryTicketResult
             * @instance
             */
            EntryTicketResult.prototype.game_wss_url = "";

            /**
             * EntryTicketResult run_server_id.
             * @member {Long} run_server_id
             * @memberof minimir.v1.EntryTicketResult
             * @instance
             */
            EntryTicketResult.prototype.run_server_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EntryTicketResult route_version.
             * @member {Long} route_version
             * @memberof minimir.v1.EntryTicketResult
             * @instance
             */
            EntryTicketResult.prototype.route_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EntryTicketResult expires_at_unix.
             * @member {Long} expires_at_unix
             * @memberof minimir.v1.EntryTicketResult
             * @instance
             */
            EntryTicketResult.prototype.expires_at_unix = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new EntryTicketResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {minimir.v1.EntryTicketResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EntryTicketResult} EntryTicketResult instance
             * @type {{
             *   (properties: minimir.v1.EntryTicketResult.$Shape): minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape;
             *   (properties?: minimir.v1.EntryTicketResult.$Properties): minimir.v1.EntryTicketResult;
             * }}
             */
            EntryTicketResult.create = function(properties) {
                return new EntryTicketResult(properties);
            };

            /**
             * Encodes the specified EntryTicketResult message. Does not implicitly {@link minimir.v1.EntryTicketResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {minimir.v1.EntryTicketResult.$Properties} message EntryTicketResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntryTicketResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.ticket != null && $Object.hasOwnProperty.call(message, "ticket") && message.ticket !== "")
                    writer.uint32(/* id 1, wireType 2 =*/10).string(message.ticket);
                if (message.game_wss_url != null && $Object.hasOwnProperty.call(message, "game_wss_url") && message.game_wss_url !== "")
                    writer.uint32(/* id 2, wireType 2 =*/18).string(message.game_wss_url);
                if (message.run_server_id != null && $Object.hasOwnProperty.call(message, "run_server_id") && (typeof message.run_server_id === "object" ? message.run_server_id.low || message.run_server_id.high : message.run_server_id !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.run_server_id);
                if (message.route_version != null && $Object.hasOwnProperty.call(message, "route_version") && (typeof message.route_version === "object" ? message.route_version.low || message.route_version.high : message.route_version !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).uint64(message.route_version);
                if (message.expires_at_unix != null && $Object.hasOwnProperty.call(message, "expires_at_unix") && (typeof message.expires_at_unix === "object" ? message.expires_at_unix.low || message.expires_at_unix.high : message.expires_at_unix !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.expires_at_unix);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EntryTicketResult message, length delimited. Does not implicitly {@link minimir.v1.EntryTicketResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {minimir.v1.EntryTicketResult.$Properties} message EntryTicketResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntryTicketResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EntryTicketResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape} EntryTicketResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntryTicketResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EntryTicketResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.ticket = value;
                            else
                                delete message.ticket;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.game_wss_url = value;
                            else
                                delete message.game_wss_url;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.run_server_id = value;
                            else
                                delete message.run_server_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.route_version = value;
                            else
                                delete message.route_version;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.expires_at_unix = value;
                            else
                                delete message.expires_at_unix;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EntryTicketResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EntryTicketResult & minimir.v1.EntryTicketResult.$Shape} EntryTicketResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntryTicketResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EntryTicketResult message.
             * @function verify
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EntryTicketResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.ticket != null && $Object.hasOwnProperty.call(message, "ticket"))
                    if (!$util.isString(message.ticket))
                        return "ticket: string expected";
                if (message.game_wss_url != null && $Object.hasOwnProperty.call(message, "game_wss_url"))
                    if (!$util.isString(message.game_wss_url))
                        return "game_wss_url: string expected";
                if (message.run_server_id != null && $Object.hasOwnProperty.call(message, "run_server_id"))
                    if (!$util.isInteger(message.run_server_id) && !(message.run_server_id && $util.isInteger(message.run_server_id.low) && $util.isInteger(message.run_server_id.high)))
                        return "run_server_id: integer|Long expected";
                if (message.route_version != null && $Object.hasOwnProperty.call(message, "route_version"))
                    if (!$util.isInteger(message.route_version) && !(message.route_version && $util.isInteger(message.route_version.low) && $util.isInteger(message.route_version.high)))
                        return "route_version: integer|Long expected";
                if (message.expires_at_unix != null && $Object.hasOwnProperty.call(message, "expires_at_unix"))
                    if (!$util.isInteger(message.expires_at_unix) && !(message.expires_at_unix && $util.isInteger(message.expires_at_unix.low) && $util.isInteger(message.expires_at_unix.high)))
                        return "expires_at_unix: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for EntryTicketResult
             * @function getTypeUrl
             * @memberof minimir.v1.EntryTicketResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EntryTicketResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EntryTicketResult";
            };

            return EntryTicketResult;
        })();

        v1.EnterLineRequest = (function() {

            /**
             * Properties of an EnterLineRequest.
             * @typedef {Object} minimir.v1.EnterLineRequest.$Properties
             * @property {string|null} [account_ticket] EnterLineRequest account_ticket
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EnterLineRequest.
             * @memberof minimir.v1
             * @interface IEnterLineRequest
             * @augments minimir.v1.EnterLineRequest.$Properties
             * @deprecated Use minimir.v1.EnterLineRequest.$Properties instead.
             */

            /**
             * Shape of an EnterLineRequest.
             * @typedef {minimir.v1.EnterLineRequest.$Properties} minimir.v1.EnterLineRequest.$Shape
             */

            /**
             * Constructs a new EnterLineRequest.
             * @memberof minimir.v1
             * @classdesc Represents an EnterLineRequest.
             * @constructor
             * @param {minimir.v1.EnterLineRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EnterLineRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EnterLineRequest account_ticket.
             * @member {string} account_ticket
             * @memberof minimir.v1.EnterLineRequest
             * @instance
             */
            EnterLineRequest.prototype.account_ticket = "";

            /**
             * Creates a new EnterLineRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {minimir.v1.EnterLineRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EnterLineRequest} EnterLineRequest instance
             * @type {{
             *   (properties: minimir.v1.EnterLineRequest.$Shape): minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape;
             *   (properties?: minimir.v1.EnterLineRequest.$Properties): minimir.v1.EnterLineRequest;
             * }}
             */
            EnterLineRequest.create = function(properties) {
                return new EnterLineRequest(properties);
            };

            /**
             * Encodes the specified EnterLineRequest message. Does not implicitly {@link minimir.v1.EnterLineRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {minimir.v1.EnterLineRequest.$Properties} message EnterLineRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterLineRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.account_ticket != null && $Object.hasOwnProperty.call(message, "account_ticket") && message.account_ticket !== "")
                    writer.uint32(/* id 1, wireType 2 =*/10).string(message.account_ticket);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EnterLineRequest message, length delimited. Does not implicitly {@link minimir.v1.EnterLineRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {minimir.v1.EnterLineRequest.$Properties} message EnterLineRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterLineRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EnterLineRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape} EnterLineRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterLineRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EnterLineRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.account_ticket = value;
                            else
                                delete message.account_ticket;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EnterLineRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterLineRequest & minimir.v1.EnterLineRequest.$Shape} EnterLineRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterLineRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EnterLineRequest message.
             * @function verify
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EnterLineRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.account_ticket != null && $Object.hasOwnProperty.call(message, "account_ticket"))
                    if (!$util.isString(message.account_ticket))
                        return "account_ticket: string expected";
                return null;
            };

            /**
             * Gets the type url for EnterLineRequest
             * @function getTypeUrl
             * @memberof minimir.v1.EnterLineRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EnterLineRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EnterLineRequest";
            };

            return EnterLineRequest;
        })();

        v1.EnterLineResult = (function() {

            /**
             * Properties of an EnterLineResult.
             * @typedef {Object} minimir.v1.EnterLineResult.$Properties
             * @property {Long|null} [scene_epoch] EnterLineResult scene_epoch
             * @property {Long|null} [baseline_id] EnterLineResult baseline_id
             * @property {Long|null} [room_id] EnterLineResult room_id
             * @property {number|null} [line_id] EnterLineResult line_id
             * @property {Long|null} [resource_version] EnterLineResult resource_version
             * @property {minimir.v1.GridPoint.$Properties|null} [position] EnterLineResult position
             * @property {number|null} [scene_kind] EnterLineResult scene_kind
             * @property {Long|null} [next_economic_command_seq] EnterLineResult next_economic_command_seq
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EnterLineResult.
             * @memberof minimir.v1
             * @interface IEnterLineResult
             * @augments minimir.v1.EnterLineResult.$Properties
             * @deprecated Use minimir.v1.EnterLineResult.$Properties instead.
             */

            /**
             * Shape of an EnterLineResult.
             * @typedef {minimir.v1.EnterLineResult.$Properties} minimir.v1.EnterLineResult.$Shape
             */

            /**
             * Constructs a new EnterLineResult.
             * @memberof minimir.v1
             * @classdesc Represents an EnterLineResult.
             * @constructor
             * @param {minimir.v1.EnterLineResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EnterLineResult = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EnterLineResult scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EnterLineResult baseline_id.
             * @member {Long} baseline_id
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.baseline_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EnterLineResult room_id.
             * @member {Long} room_id
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.room_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EnterLineResult line_id.
             * @member {number} line_id
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.line_id = 0;

            /**
             * EnterLineResult resource_version.
             * @member {Long} resource_version
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.resource_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EnterLineResult position.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} position
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.position = null;

            /**
             * EnterLineResult scene_kind.
             * @member {number} scene_kind
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.scene_kind = 0;

            /**
             * EnterLineResult next_economic_command_seq.
             * @member {Long} next_economic_command_seq
             * @memberof minimir.v1.EnterLineResult
             * @instance
             */
            EnterLineResult.prototype.next_economic_command_seq = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new EnterLineResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {minimir.v1.EnterLineResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EnterLineResult} EnterLineResult instance
             * @type {{
             *   (properties: minimir.v1.EnterLineResult.$Shape): minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape;
             *   (properties?: minimir.v1.EnterLineResult.$Properties): minimir.v1.EnterLineResult;
             * }}
             */
            EnterLineResult.create = function(properties) {
                return new EnterLineResult(properties);
            };

            /**
             * Encodes the specified EnterLineResult message. Does not implicitly {@link minimir.v1.EnterLineResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {minimir.v1.EnterLineResult.$Properties} message EnterLineResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterLineResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id") && (typeof message.baseline_id === "object" ? message.baseline_id.low || message.baseline_id.high : message.baseline_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.baseline_id);
                if (message.room_id != null && $Object.hasOwnProperty.call(message, "room_id") && (typeof message.room_id === "object" ? message.room_id.low || message.room_id.high : message.room_id !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.room_id);
                if (message.line_id != null && $Object.hasOwnProperty.call(message, "line_id") && message.line_id !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.line_id);
                if (message.resource_version != null && $Object.hasOwnProperty.call(message, "resource_version") && (typeof message.resource_version === "object" ? message.resource_version.low || message.resource_version.high : message.resource_version !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.resource_version);
                if (message.position != null && $Object.hasOwnProperty.call(message, "position"))
                    $root.minimir.v1.GridPoint.encode(message.position, writer.uint32(/* id 6, wireType 2 =*/50).fork(), _depth + 1).ldelim();
                if (message.scene_kind != null && $Object.hasOwnProperty.call(message, "scene_kind") && message.scene_kind !== 0)
                    writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.scene_kind);
                if (message.next_economic_command_seq != null && $Object.hasOwnProperty.call(message, "next_economic_command_seq") && (typeof message.next_economic_command_seq === "object" ? message.next_economic_command_seq.low || message.next_economic_command_seq.high : message.next_economic_command_seq !== 0))
                    writer.uint32(/* id 8, wireType 0 =*/64).uint64(message.next_economic_command_seq);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EnterLineResult message, length delimited. Does not implicitly {@link minimir.v1.EnterLineResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {minimir.v1.EnterLineResult.$Properties} message EnterLineResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterLineResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EnterLineResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape} EnterLineResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterLineResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EnterLineResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.baseline_id = value;
                            else
                                delete message.baseline_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.room_id = value;
                            else
                                delete message.room_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.line_id = value;
                            else
                                delete message.line_id;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.resource_version = value;
                            else
                                delete message.resource_version;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            message.position = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.position);
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.scene_kind = value;
                            else
                                delete message.scene_kind;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.next_economic_command_seq = value;
                            else
                                delete message.next_economic_command_seq;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EnterLineResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterLineResult & minimir.v1.EnterLineResult.$Shape} EnterLineResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterLineResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EnterLineResult message.
             * @function verify
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EnterLineResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id"))
                    if (!$util.isInteger(message.baseline_id) && !(message.baseline_id && $util.isInteger(message.baseline_id.low) && $util.isInteger(message.baseline_id.high)))
                        return "baseline_id: integer|Long expected";
                if (message.room_id != null && $Object.hasOwnProperty.call(message, "room_id"))
                    if (!$util.isInteger(message.room_id) && !(message.room_id && $util.isInteger(message.room_id.low) && $util.isInteger(message.room_id.high)))
                        return "room_id: integer|Long expected";
                if (message.line_id != null && $Object.hasOwnProperty.call(message, "line_id"))
                    if (!$util.isInteger(message.line_id))
                        return "line_id: integer expected";
                if (message.resource_version != null && $Object.hasOwnProperty.call(message, "resource_version"))
                    if (!$util.isInteger(message.resource_version) && !(message.resource_version && $util.isInteger(message.resource_version.low) && $util.isInteger(message.resource_version.high)))
                        return "resource_version: integer|Long expected";
                if (message.position != null && $Object.hasOwnProperty.call(message, "position")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.position, _depth + 1);
                    if (error)
                        return "position." + error;
                }
                if (message.scene_kind != null && $Object.hasOwnProperty.call(message, "scene_kind"))
                    if (!$util.isInteger(message.scene_kind))
                        return "scene_kind: integer expected";
                if (message.next_economic_command_seq != null && $Object.hasOwnProperty.call(message, "next_economic_command_seq"))
                    if (!$util.isInteger(message.next_economic_command_seq) && !(message.next_economic_command_seq && $util.isInteger(message.next_economic_command_seq.low) && $util.isInteger(message.next_economic_command_seq.high)))
                        return "next_economic_command_seq: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for EnterLineResult
             * @function getTypeUrl
             * @memberof minimir.v1.EnterLineResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EnterLineResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EnterLineResult";
            };

            return EnterLineResult;
        })();

        v1.EnterWorldAck = (function() {

            /**
             * Properties of an EnterWorldAck.
             * @typedef {Object} minimir.v1.EnterWorldAck.$Properties
             * @property {Long|null} [scene_epoch] EnterWorldAck scene_epoch
             * @property {Long|null} [baseline_id] EnterWorldAck baseline_id
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EnterWorldAck.
             * @memberof minimir.v1
             * @interface IEnterWorldAck
             * @augments minimir.v1.EnterWorldAck.$Properties
             * @deprecated Use minimir.v1.EnterWorldAck.$Properties instead.
             */

            /**
             * Shape of an EnterWorldAck.
             * @typedef {minimir.v1.EnterWorldAck.$Properties} minimir.v1.EnterWorldAck.$Shape
             */

            /**
             * Constructs a new EnterWorldAck.
             * @memberof minimir.v1
             * @classdesc Represents an EnterWorldAck.
             * @constructor
             * @param {minimir.v1.EnterWorldAck.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EnterWorldAck = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EnterWorldAck scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.EnterWorldAck
             * @instance
             */
            EnterWorldAck.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EnterWorldAck baseline_id.
             * @member {Long} baseline_id
             * @memberof minimir.v1.EnterWorldAck
             * @instance
             */
            EnterWorldAck.prototype.baseline_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new EnterWorldAck instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {minimir.v1.EnterWorldAck.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EnterWorldAck} EnterWorldAck instance
             * @type {{
             *   (properties: minimir.v1.EnterWorldAck.$Shape): minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape;
             *   (properties?: minimir.v1.EnterWorldAck.$Properties): minimir.v1.EnterWorldAck;
             * }}
             */
            EnterWorldAck.create = function(properties) {
                return new EnterWorldAck(properties);
            };

            /**
             * Encodes the specified EnterWorldAck message. Does not implicitly {@link minimir.v1.EnterWorldAck.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {minimir.v1.EnterWorldAck.$Properties} message EnterWorldAck message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterWorldAck.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id") && (typeof message.baseline_id === "object" ? message.baseline_id.low || message.baseline_id.high : message.baseline_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.baseline_id);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EnterWorldAck message, length delimited. Does not implicitly {@link minimir.v1.EnterWorldAck.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {minimir.v1.EnterWorldAck.$Properties} message EnterWorldAck message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EnterWorldAck.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EnterWorldAck message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape} EnterWorldAck
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterWorldAck.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EnterWorldAck();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.baseline_id = value;
                            else
                                delete message.baseline_id;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EnterWorldAck message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EnterWorldAck & minimir.v1.EnterWorldAck.$Shape} EnterWorldAck
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EnterWorldAck.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EnterWorldAck message.
             * @function verify
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EnterWorldAck.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id"))
                    if (!$util.isInteger(message.baseline_id) && !(message.baseline_id && $util.isInteger(message.baseline_id.low) && $util.isInteger(message.baseline_id.high)))
                        return "baseline_id: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for EnterWorldAck
             * @function getTypeUrl
             * @memberof minimir.v1.EnterWorldAck
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EnterWorldAck.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EnterWorldAck";
            };

            return EnterWorldAck;
        })();

        v1.SyncReady = (function() {

            /**
             * Properties of a SyncReady.
             * @typedef {Object} minimir.v1.SyncReady.$Properties
             * @property {Long|null} [scene_epoch] SyncReady scene_epoch
             * @property {Long|null} [baseline_id] SyncReady baseline_id
             * @property {Long|null} [next_economic_command_seq] SyncReady next_economic_command_seq
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SyncReady.
             * @memberof minimir.v1
             * @interface ISyncReady
             * @augments minimir.v1.SyncReady.$Properties
             * @deprecated Use minimir.v1.SyncReady.$Properties instead.
             */

            /**
             * Shape of a SyncReady.
             * @typedef {minimir.v1.SyncReady.$Properties} minimir.v1.SyncReady.$Shape
             */

            /**
             * Constructs a new SyncReady.
             * @memberof minimir.v1
             * @classdesc Represents a SyncReady.
             * @constructor
             * @param {minimir.v1.SyncReady.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SyncReady = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SyncReady scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.SyncReady
             * @instance
             */
            SyncReady.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SyncReady baseline_id.
             * @member {Long} baseline_id
             * @memberof minimir.v1.SyncReady
             * @instance
             */
            SyncReady.prototype.baseline_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SyncReady next_economic_command_seq.
             * @member {Long} next_economic_command_seq
             * @memberof minimir.v1.SyncReady
             * @instance
             */
            SyncReady.prototype.next_economic_command_seq = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new SyncReady instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {minimir.v1.SyncReady.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SyncReady} SyncReady instance
             * @type {{
             *   (properties: minimir.v1.SyncReady.$Shape): minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape;
             *   (properties?: minimir.v1.SyncReady.$Properties): minimir.v1.SyncReady;
             * }}
             */
            SyncReady.create = function(properties) {
                return new SyncReady(properties);
            };

            /**
             * Encodes the specified SyncReady message. Does not implicitly {@link minimir.v1.SyncReady.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {minimir.v1.SyncReady.$Properties} message SyncReady message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SyncReady.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id") && (typeof message.baseline_id === "object" ? message.baseline_id.low || message.baseline_id.high : message.baseline_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.baseline_id);
                if (message.next_economic_command_seq != null && $Object.hasOwnProperty.call(message, "next_economic_command_seq") && (typeof message.next_economic_command_seq === "object" ? message.next_economic_command_seq.low || message.next_economic_command_seq.high : message.next_economic_command_seq !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.next_economic_command_seq);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SyncReady message, length delimited. Does not implicitly {@link minimir.v1.SyncReady.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {minimir.v1.SyncReady.$Properties} message SyncReady message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SyncReady.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SyncReady message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape} SyncReady
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SyncReady.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SyncReady();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.baseline_id = value;
                            else
                                delete message.baseline_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.next_economic_command_seq = value;
                            else
                                delete message.next_economic_command_seq;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SyncReady message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SyncReady & minimir.v1.SyncReady.$Shape} SyncReady
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SyncReady.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SyncReady message.
             * @function verify
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SyncReady.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id"))
                    if (!$util.isInteger(message.baseline_id) && !(message.baseline_id && $util.isInteger(message.baseline_id.low) && $util.isInteger(message.baseline_id.high)))
                        return "baseline_id: integer|Long expected";
                if (message.next_economic_command_seq != null && $Object.hasOwnProperty.call(message, "next_economic_command_seq"))
                    if (!$util.isInteger(message.next_economic_command_seq) && !(message.next_economic_command_seq && $util.isInteger(message.next_economic_command_seq.low) && $util.isInteger(message.next_economic_command_seq.high)))
                        return "next_economic_command_seq: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for SyncReady
             * @function getTypeUrl
             * @memberof minimir.v1.SyncReady
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SyncReady.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SyncReady";
            };

            return SyncReady;
        })();

        v1.EntityState = (function() {

            /**
             * Properties of an EntityState.
             * @typedef {Object} minimir.v1.EntityState.$Properties
             * @property {number|null} [entity_handle] EntityState entity_handle
             * @property {number|null} [entity_kind] EntityState entity_kind
             * @property {number|null} [definition_id] EntityState definition_id
             * @property {minimir.v1.GridPoint.$Properties|null} [position] EntityState position
             * @property {number|null} [direction] EntityState direction
             * @property {Long|null} [hp] EntityState hp
             * @property {Long|null} [hp_max] EntityState hp_max
             * @property {number|null} [action] EntityState action
             * @property {Long|null} [mp] EntityState mp
             * @property {Long|null} [mp_max] EntityState mp_max
             * @property {number|null} [move_interval_ms] EntityState move_interval_ms
             * @property {number|null} [attack_sequence] EntityState attack_sequence
             * @property {number|null} [cast_sequence] EntityState cast_sequence
             * @property {number|null} [cast_skill_id] EntityState cast_skill_id
             * @property {Long|null} [life] EntityState life
             * @property {number|null} [attack_interval_ms] EntityState attack_interval_ms
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EntityState.
             * @memberof minimir.v1
             * @interface IEntityState
             * @augments minimir.v1.EntityState.$Properties
             * @deprecated Use minimir.v1.EntityState.$Properties instead.
             */

            /**
             * Shape of an EntityState.
             * @typedef {minimir.v1.EntityState.$Properties} minimir.v1.EntityState.$Shape
             */

            /**
             * Constructs a new EntityState.
             * @memberof minimir.v1
             * @classdesc Represents an EntityState.
             * @constructor
             * @param {minimir.v1.EntityState.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EntityState = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EntityState entity_handle.
             * @member {number} entity_handle
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.entity_handle = 0;

            /**
             * EntityState entity_kind.
             * @member {number} entity_kind
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.entity_kind = 0;

            /**
             * EntityState definition_id.
             * @member {number} definition_id
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.definition_id = 0;

            /**
             * EntityState position.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} position
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.position = null;

            /**
             * EntityState direction.
             * @member {number} direction
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.direction = 0;

            /**
             * EntityState hp.
             * @member {Long} hp
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.hp = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * EntityState hp_max.
             * @member {Long} hp_max
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.hp_max = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * EntityState action.
             * @member {number} action
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.action = 0;

            /**
             * EntityState mp.
             * @member {Long} mp
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.mp = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * EntityState mp_max.
             * @member {Long} mp_max
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.mp_max = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * EntityState move_interval_ms.
             * @member {number} move_interval_ms
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.move_interval_ms = 0;

            /**
             * EntityState attack_sequence.
             * @member {number} attack_sequence
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.attack_sequence = 0;

            /**
             * EntityState cast_sequence.
             * @member {number} cast_sequence
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.cast_sequence = 0;

            /**
             * EntityState cast_skill_id.
             * @member {number} cast_skill_id
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.cast_skill_id = 0;

            /**
             * EntityState life.
             * @member {Long} life
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.life = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * EntityState attack_interval_ms.
             * @member {number} attack_interval_ms
             * @memberof minimir.v1.EntityState
             * @instance
             */
            EntityState.prototype.attack_interval_ms = 0;

            /**
             * Creates a new EntityState instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EntityState
             * @static
             * @param {minimir.v1.EntityState.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EntityState} EntityState instance
             * @type {{
             *   (properties: minimir.v1.EntityState.$Shape): minimir.v1.EntityState & minimir.v1.EntityState.$Shape;
             *   (properties?: minimir.v1.EntityState.$Properties): minimir.v1.EntityState;
             * }}
             */
            EntityState.create = function(properties) {
                return new EntityState(properties);
            };

            /**
             * Encodes the specified EntityState message. Does not implicitly {@link minimir.v1.EntityState.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EntityState
             * @static
             * @param {minimir.v1.EntityState.$Properties} message EntityState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntityState.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.entity_handle != null && $Object.hasOwnProperty.call(message, "entity_handle") && message.entity_handle !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.entity_handle);
                if (message.entity_kind != null && $Object.hasOwnProperty.call(message, "entity_kind") && message.entity_kind !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.entity_kind);
                if (message.definition_id != null && $Object.hasOwnProperty.call(message, "definition_id") && message.definition_id !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.definition_id);
                if (message.position != null && $Object.hasOwnProperty.call(message, "position"))
                    $root.minimir.v1.GridPoint.encode(message.position, writer.uint32(/* id 4, wireType 2 =*/34).fork(), _depth + 1).ldelim();
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction") && message.direction !== 0)
                    writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.direction);
                if (message.hp != null && $Object.hasOwnProperty.call(message, "hp") && (typeof message.hp === "object" ? message.hp.low || message.hp.high : message.hp !== 0))
                    writer.uint32(/* id 6, wireType 0 =*/48).int64(message.hp);
                if (message.hp_max != null && $Object.hasOwnProperty.call(message, "hp_max") && (typeof message.hp_max === "object" ? message.hp_max.low || message.hp_max.high : message.hp_max !== 0))
                    writer.uint32(/* id 7, wireType 0 =*/56).int64(message.hp_max);
                if (message.action != null && $Object.hasOwnProperty.call(message, "action") && message.action !== 0)
                    writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.action);
                if (message.mp != null && $Object.hasOwnProperty.call(message, "mp") && (typeof message.mp === "object" ? message.mp.low || message.mp.high : message.mp !== 0))
                    writer.uint32(/* id 9, wireType 0 =*/72).int64(message.mp);
                if (message.mp_max != null && $Object.hasOwnProperty.call(message, "mp_max") && (typeof message.mp_max === "object" ? message.mp_max.low || message.mp_max.high : message.mp_max !== 0))
                    writer.uint32(/* id 10, wireType 0 =*/80).int64(message.mp_max);
                if (message.move_interval_ms != null && $Object.hasOwnProperty.call(message, "move_interval_ms") && message.move_interval_ms !== 0)
                    writer.uint32(/* id 11, wireType 0 =*/88).uint32(message.move_interval_ms);
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence") && message.attack_sequence !== 0)
                    writer.uint32(/* id 12, wireType 0 =*/96).uint32(message.attack_sequence);
                if (message.cast_sequence != null && $Object.hasOwnProperty.call(message, "cast_sequence") && message.cast_sequence !== 0)
                    writer.uint32(/* id 13, wireType 0 =*/104).uint32(message.cast_sequence);
                if (message.cast_skill_id != null && $Object.hasOwnProperty.call(message, "cast_skill_id") && message.cast_skill_id !== 0)
                    writer.uint32(/* id 14, wireType 0 =*/112).uint32(message.cast_skill_id);
                if (message.life != null && $Object.hasOwnProperty.call(message, "life") && (typeof message.life === "object" ? message.life.low || message.life.high : message.life !== 0))
                    writer.uint32(/* id 15, wireType 0 =*/120).uint64(message.life);
                if (message.attack_interval_ms != null && $Object.hasOwnProperty.call(message, "attack_interval_ms") && message.attack_interval_ms !== 0)
                    writer.uint32(/* id 16, wireType 0 =*/128).uint32(message.attack_interval_ms);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EntityState message, length delimited. Does not implicitly {@link minimir.v1.EntityState.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EntityState
             * @static
             * @param {minimir.v1.EntityState.$Properties} message EntityState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EntityState.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EntityState message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EntityState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EntityState & minimir.v1.EntityState.$Shape} EntityState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntityState.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EntityState();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.entity_handle = value;
                            else
                                delete message.entity_handle;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.entity_kind = value;
                            else
                                delete message.entity_kind;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.definition_id = value;
                            else
                                delete message.definition_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 2)
                                break;
                            message.position = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.position);
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.direction = value;
                            else
                                delete message.direction;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.hp = value;
                            else
                                delete message.hp;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.hp_max = value;
                            else
                                delete message.hp_max;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.action = value;
                            else
                                delete message.action;
                            continue;
                        }
                    case 9: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.mp = value;
                            else
                                delete message.mp;
                            continue;
                        }
                    case 10: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.mp_max = value;
                            else
                                delete message.mp_max;
                            continue;
                        }
                    case 11: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.move_interval_ms = value;
                            else
                                delete message.move_interval_ms;
                            continue;
                        }
                    case 12: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.attack_sequence = value;
                            else
                                delete message.attack_sequence;
                            continue;
                        }
                    case 13: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.cast_sequence = value;
                            else
                                delete message.cast_sequence;
                            continue;
                        }
                    case 14: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.cast_skill_id = value;
                            else
                                delete message.cast_skill_id;
                            continue;
                        }
                    case 15: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.life = value;
                            else
                                delete message.life;
                            continue;
                        }
                    case 16: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.attack_interval_ms = value;
                            else
                                delete message.attack_interval_ms;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EntityState message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EntityState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EntityState & minimir.v1.EntityState.$Shape} EntityState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EntityState.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EntityState message.
             * @function verify
             * @memberof minimir.v1.EntityState
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EntityState.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.entity_handle != null && $Object.hasOwnProperty.call(message, "entity_handle"))
                    if (!$util.isInteger(message.entity_handle))
                        return "entity_handle: integer expected";
                if (message.entity_kind != null && $Object.hasOwnProperty.call(message, "entity_kind"))
                    if (!$util.isInteger(message.entity_kind))
                        return "entity_kind: integer expected";
                if (message.definition_id != null && $Object.hasOwnProperty.call(message, "definition_id"))
                    if (!$util.isInteger(message.definition_id))
                        return "definition_id: integer expected";
                if (message.position != null && $Object.hasOwnProperty.call(message, "position")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.position, _depth + 1);
                    if (error)
                        return "position." + error;
                }
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction"))
                    if (!$util.isInteger(message.direction))
                        return "direction: integer expected";
                if (message.hp != null && $Object.hasOwnProperty.call(message, "hp"))
                    if (!$util.isInteger(message.hp) && !(message.hp && $util.isInteger(message.hp.low) && $util.isInteger(message.hp.high)))
                        return "hp: integer|Long expected";
                if (message.hp_max != null && $Object.hasOwnProperty.call(message, "hp_max"))
                    if (!$util.isInteger(message.hp_max) && !(message.hp_max && $util.isInteger(message.hp_max.low) && $util.isInteger(message.hp_max.high)))
                        return "hp_max: integer|Long expected";
                if (message.action != null && $Object.hasOwnProperty.call(message, "action"))
                    if (!$util.isInteger(message.action))
                        return "action: integer expected";
                if (message.mp != null && $Object.hasOwnProperty.call(message, "mp"))
                    if (!$util.isInteger(message.mp) && !(message.mp && $util.isInteger(message.mp.low) && $util.isInteger(message.mp.high)))
                        return "mp: integer|Long expected";
                if (message.mp_max != null && $Object.hasOwnProperty.call(message, "mp_max"))
                    if (!$util.isInteger(message.mp_max) && !(message.mp_max && $util.isInteger(message.mp_max.low) && $util.isInteger(message.mp_max.high)))
                        return "mp_max: integer|Long expected";
                if (message.move_interval_ms != null && $Object.hasOwnProperty.call(message, "move_interval_ms"))
                    if (!$util.isInteger(message.move_interval_ms))
                        return "move_interval_ms: integer expected";
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence"))
                    if (!$util.isInteger(message.attack_sequence))
                        return "attack_sequence: integer expected";
                if (message.cast_sequence != null && $Object.hasOwnProperty.call(message, "cast_sequence"))
                    if (!$util.isInteger(message.cast_sequence))
                        return "cast_sequence: integer expected";
                if (message.cast_skill_id != null && $Object.hasOwnProperty.call(message, "cast_skill_id"))
                    if (!$util.isInteger(message.cast_skill_id))
                        return "cast_skill_id: integer expected";
                if (message.life != null && $Object.hasOwnProperty.call(message, "life"))
                    if (!$util.isInteger(message.life) && !(message.life && $util.isInteger(message.life.low) && $util.isInteger(message.life.high)))
                        return "life: integer|Long expected";
                if (message.attack_interval_ms != null && $Object.hasOwnProperty.call(message, "attack_interval_ms"))
                    if (!$util.isInteger(message.attack_interval_ms))
                        return "attack_interval_ms: integer expected";
                return null;
            };

            /**
             * Gets the type url for EntityState
             * @function getTypeUrl
             * @memberof minimir.v1.EntityState
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EntityState.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EntityState";
            };

            return EntityState;
        })();

        v1.SceneBaselineChunk = (function() {

            /**
             * Properties of a SceneBaselineChunk.
             * @typedef {Object} minimir.v1.SceneBaselineChunk.$Properties
             * @property {Long|null} [scene_epoch] SceneBaselineChunk scene_epoch
             * @property {Long|null} [baseline_id] SceneBaselineChunk baseline_id
             * @property {number|null} [chunk_index] SceneBaselineChunk chunk_index
             * @property {number|null} [chunk_count] SceneBaselineChunk chunk_count
             * @property {Long|null} [state_sequence] SceneBaselineChunk state_sequence
             * @property {Array.<minimir.v1.EntityState.$Properties>|null} [entities] SceneBaselineChunk entities
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SceneBaselineChunk.
             * @memberof minimir.v1
             * @interface ISceneBaselineChunk
             * @augments minimir.v1.SceneBaselineChunk.$Properties
             * @deprecated Use minimir.v1.SceneBaselineChunk.$Properties instead.
             */

            /**
             * Shape of a SceneBaselineChunk.
             * @typedef {minimir.v1.SceneBaselineChunk.$Properties} minimir.v1.SceneBaselineChunk.$Shape
             */

            /**
             * Constructs a new SceneBaselineChunk.
             * @memberof minimir.v1
             * @classdesc Represents a SceneBaselineChunk.
             * @constructor
             * @param {minimir.v1.SceneBaselineChunk.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SceneBaselineChunk = function (properties) {
                this.entities = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SceneBaselineChunk scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SceneBaselineChunk baseline_id.
             * @member {Long} baseline_id
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.baseline_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SceneBaselineChunk chunk_index.
             * @member {number} chunk_index
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.chunk_index = 0;

            /**
             * SceneBaselineChunk chunk_count.
             * @member {number} chunk_count
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.chunk_count = 0;

            /**
             * SceneBaselineChunk state_sequence.
             * @member {Long} state_sequence
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.state_sequence = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SceneBaselineChunk entities.
             * @member {Array.<minimir.v1.EntityState.$Properties>} entities
             * @memberof minimir.v1.SceneBaselineChunk
             * @instance
             */
            SceneBaselineChunk.prototype.entities = $util.emptyArray;

            /**
             * Creates a new SceneBaselineChunk instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {minimir.v1.SceneBaselineChunk.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SceneBaselineChunk} SceneBaselineChunk instance
             * @type {{
             *   (properties: minimir.v1.SceneBaselineChunk.$Shape): minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape;
             *   (properties?: minimir.v1.SceneBaselineChunk.$Properties): minimir.v1.SceneBaselineChunk;
             * }}
             */
            SceneBaselineChunk.create = function(properties) {
                return new SceneBaselineChunk(properties);
            };

            /**
             * Encodes the specified SceneBaselineChunk message. Does not implicitly {@link minimir.v1.SceneBaselineChunk.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {minimir.v1.SceneBaselineChunk.$Properties} message SceneBaselineChunk message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SceneBaselineChunk.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id") && (typeof message.baseline_id === "object" ? message.baseline_id.low || message.baseline_id.high : message.baseline_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.baseline_id);
                if (message.chunk_index != null && $Object.hasOwnProperty.call(message, "chunk_index") && message.chunk_index !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.chunk_index);
                if (message.chunk_count != null && $Object.hasOwnProperty.call(message, "chunk_count") && message.chunk_count !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.chunk_count);
                if (message.state_sequence != null && $Object.hasOwnProperty.call(message, "state_sequence") && (typeof message.state_sequence === "object" ? message.state_sequence.low || message.state_sequence.high : message.state_sequence !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.state_sequence);
                if (message.entities != null && message.entities.length)
                    for (var i = 0; i < message.entities.length; ++i)
                        $root.minimir.v1.EntityState.encode(message.entities[i], writer.uint32(/* id 6, wireType 2 =*/50).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SceneBaselineChunk message, length delimited. Does not implicitly {@link minimir.v1.SceneBaselineChunk.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {minimir.v1.SceneBaselineChunk.$Properties} message SceneBaselineChunk message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SceneBaselineChunk.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SceneBaselineChunk message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape} SceneBaselineChunk
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SceneBaselineChunk.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SceneBaselineChunk();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.baseline_id = value;
                            else
                                delete message.baseline_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.chunk_index = value;
                            else
                                delete message.chunk_index;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.chunk_count = value;
                            else
                                delete message.chunk_count;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.state_sequence = value;
                            else
                                delete message.state_sequence;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            if (!(message.entities && message.entities.length))
                                message.entities = [];
                            message.entities.push($root.minimir.v1.EntityState.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SceneBaselineChunk message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SceneBaselineChunk & minimir.v1.SceneBaselineChunk.$Shape} SceneBaselineChunk
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SceneBaselineChunk.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SceneBaselineChunk message.
             * @function verify
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SceneBaselineChunk.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.baseline_id != null && $Object.hasOwnProperty.call(message, "baseline_id"))
                    if (!$util.isInteger(message.baseline_id) && !(message.baseline_id && $util.isInteger(message.baseline_id.low) && $util.isInteger(message.baseline_id.high)))
                        return "baseline_id: integer|Long expected";
                if (message.chunk_index != null && $Object.hasOwnProperty.call(message, "chunk_index"))
                    if (!$util.isInteger(message.chunk_index))
                        return "chunk_index: integer expected";
                if (message.chunk_count != null && $Object.hasOwnProperty.call(message, "chunk_count"))
                    if (!$util.isInteger(message.chunk_count))
                        return "chunk_count: integer expected";
                if (message.state_sequence != null && $Object.hasOwnProperty.call(message, "state_sequence"))
                    if (!$util.isInteger(message.state_sequence) && !(message.state_sequence && $util.isInteger(message.state_sequence.low) && $util.isInteger(message.state_sequence.high)))
                        return "state_sequence: integer|Long expected";
                if (message.entities != null && $Object.hasOwnProperty.call(message, "entities")) {
                    if (!$Array.isArray(message.entities))
                        return "entities: array expected";
                    for (var i = 0; i < message.entities.length; ++i) {
                        var error = $root.minimir.v1.EntityState.verify(message.entities[i], _depth + 1);
                        if (error)
                            return "entities." + error;
                    }
                }
                return null;
            };

            /**
             * Gets the type url for SceneBaselineChunk
             * @function getTypeUrl
             * @memberof minimir.v1.SceneBaselineChunk
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SceneBaselineChunk.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SceneBaselineChunk";
            };

            return SceneBaselineChunk;
        })();

        v1.MoveIntent = (function() {

            /**
             * Properties of a MoveIntent.
             * @typedef {Object} minimir.v1.MoveIntent.$Properties
             * @property {Long|null} [scene_epoch] MoveIntent scene_epoch
             * @property {number|null} [input_sequence] MoveIntent input_sequence
             * @property {minimir.v1.GridPoint.$Properties|null} [destination] MoveIntent destination
             * @property {number|null} [direction] MoveIntent direction
             * @property {boolean|null} [release_joystick] MoveIntent release_joystick
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a MoveIntent.
             * @memberof minimir.v1
             * @interface IMoveIntent
             * @augments minimir.v1.MoveIntent.$Properties
             * @deprecated Use minimir.v1.MoveIntent.$Properties instead.
             */

            /**
             * Shape of a MoveIntent.
             * @typedef {minimir.v1.MoveIntent.$Properties} minimir.v1.MoveIntent.$Shape
             */

            /**
             * Constructs a new MoveIntent.
             * @memberof minimir.v1
             * @classdesc Represents a MoveIntent.
             * @constructor
             * @param {minimir.v1.MoveIntent.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var MoveIntent = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * MoveIntent scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.MoveIntent
             * @instance
             */
            MoveIntent.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * MoveIntent input_sequence.
             * @member {number} input_sequence
             * @memberof minimir.v1.MoveIntent
             * @instance
             */
            MoveIntent.prototype.input_sequence = 0;

            /**
             * MoveIntent destination.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} destination
             * @memberof minimir.v1.MoveIntent
             * @instance
             */
            MoveIntent.prototype.destination = null;

            /**
             * MoveIntent direction.
             * @member {number} direction
             * @memberof minimir.v1.MoveIntent
             * @instance
             */
            MoveIntent.prototype.direction = 0;

            /**
             * MoveIntent release_joystick.
             * @member {boolean} release_joystick
             * @memberof minimir.v1.MoveIntent
             * @instance
             */
            MoveIntent.prototype.release_joystick = false;

            /**
             * Creates a new MoveIntent instance using the specified properties.
             * @function create
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {minimir.v1.MoveIntent.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.MoveIntent} MoveIntent instance
             * @type {{
             *   (properties: minimir.v1.MoveIntent.$Shape): minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape;
             *   (properties?: minimir.v1.MoveIntent.$Properties): minimir.v1.MoveIntent;
             * }}
             */
            MoveIntent.create = function(properties) {
                return new MoveIntent(properties);
            };

            /**
             * Encodes the specified MoveIntent message. Does not implicitly {@link minimir.v1.MoveIntent.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {minimir.v1.MoveIntent.$Properties} message MoveIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            MoveIntent.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence") && message.input_sequence !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.input_sequence);
                if (message.destination != null && $Object.hasOwnProperty.call(message, "destination"))
                    $root.minimir.v1.GridPoint.encode(message.destination, writer.uint32(/* id 3, wireType 2 =*/26).fork(), _depth + 1).ldelim();
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction") && message.direction !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.direction);
                if (message.release_joystick != null && $Object.hasOwnProperty.call(message, "release_joystick") && message.release_joystick !== false)
                    writer.uint32(/* id 5, wireType 0 =*/40).bool(message.release_joystick);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified MoveIntent message, length delimited. Does not implicitly {@link minimir.v1.MoveIntent.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {minimir.v1.MoveIntent.$Properties} message MoveIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            MoveIntent.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a MoveIntent message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape} MoveIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            MoveIntent.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.MoveIntent();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.input_sequence = value;
                            else
                                delete message.input_sequence;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 2)
                                break;
                            message.destination = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.destination);
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.direction = value;
                            else
                                delete message.direction;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.release_joystick = value;
                            else
                                delete message.release_joystick;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a MoveIntent message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.MoveIntent & minimir.v1.MoveIntent.$Shape} MoveIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            MoveIntent.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a MoveIntent message.
             * @function verify
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            MoveIntent.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence"))
                    if (!$util.isInteger(message.input_sequence))
                        return "input_sequence: integer expected";
                if (message.destination != null && $Object.hasOwnProperty.call(message, "destination")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.destination, _depth + 1);
                    if (error)
                        return "destination." + error;
                }
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction"))
                    if (!$util.isInteger(message.direction))
                        return "direction: integer expected";
                if (message.release_joystick != null && $Object.hasOwnProperty.call(message, "release_joystick"))
                    if (typeof message.release_joystick !== "boolean")
                        return "release_joystick: boolean expected";
                return null;
            };

            /**
             * Gets the type url for MoveIntent
             * @function getTypeUrl
             * @memberof minimir.v1.MoveIntent
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            MoveIntent.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.MoveIntent";
            };

            return MoveIntent;
        })();

        v1.CastIntent = (function() {

            /**
             * Properties of a CastIntent.
             * @typedef {Object} minimir.v1.CastIntent.$Properties
             * @property {Long|null} [scene_epoch] CastIntent scene_epoch
             * @property {number|null} [input_sequence] CastIntent input_sequence
             * @property {number|null} [skill_id] CastIntent skill_id
             * @property {number|null} [target_handle] CastIntent target_handle
             * @property {minimir.v1.GridPoint.$Properties|null} [target_grid] CastIntent target_grid
             * @property {number|null} [direction] CastIntent direction
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a CastIntent.
             * @memberof minimir.v1
             * @interface ICastIntent
             * @augments minimir.v1.CastIntent.$Properties
             * @deprecated Use minimir.v1.CastIntent.$Properties instead.
             */

            /**
             * Shape of a CastIntent.
             * @typedef {minimir.v1.CastIntent.$Properties} minimir.v1.CastIntent.$Shape
             */

            /**
             * Constructs a new CastIntent.
             * @memberof minimir.v1
             * @classdesc Represents a CastIntent.
             * @constructor
             * @param {minimir.v1.CastIntent.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var CastIntent = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * CastIntent scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CastIntent input_sequence.
             * @member {number} input_sequence
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.input_sequence = 0;

            /**
             * CastIntent skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.skill_id = 0;

            /**
             * CastIntent target_handle.
             * @member {number} target_handle
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.target_handle = 0;

            /**
             * CastIntent target_grid.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} target_grid
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.target_grid = null;

            /**
             * CastIntent direction.
             * @member {number} direction
             * @memberof minimir.v1.CastIntent
             * @instance
             */
            CastIntent.prototype.direction = 0;

            /**
             * Creates a new CastIntent instance using the specified properties.
             * @function create
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {minimir.v1.CastIntent.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.CastIntent} CastIntent instance
             * @type {{
             *   (properties: minimir.v1.CastIntent.$Shape): minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape;
             *   (properties?: minimir.v1.CastIntent.$Properties): minimir.v1.CastIntent;
             * }}
             */
            CastIntent.create = function(properties) {
                return new CastIntent(properties);
            };

            /**
             * Encodes the specified CastIntent message. Does not implicitly {@link minimir.v1.CastIntent.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {minimir.v1.CastIntent.$Properties} message CastIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CastIntent.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence") && message.input_sequence !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.input_sequence);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.skill_id);
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle") && message.target_handle !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.target_handle);
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid"))
                    $root.minimir.v1.GridPoint.encode(message.target_grid, writer.uint32(/* id 5, wireType 2 =*/42).fork(), _depth + 1).ldelim();
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction") && message.direction !== 0)
                    writer.uint32(/* id 6, wireType 0 =*/48).uint32(message.direction);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified CastIntent message, length delimited. Does not implicitly {@link minimir.v1.CastIntent.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {minimir.v1.CastIntent.$Properties} message CastIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CastIntent.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a CastIntent message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape} CastIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CastIntent.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.CastIntent();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.input_sequence = value;
                            else
                                delete message.input_sequence;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.target_handle = value;
                            else
                                delete message.target_handle;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 2)
                                break;
                            message.target_grid = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.target_grid);
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.direction = value;
                            else
                                delete message.direction;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a CastIntent message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.CastIntent & minimir.v1.CastIntent.$Shape} CastIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CastIntent.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a CastIntent message.
             * @function verify
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            CastIntent.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence"))
                    if (!$util.isInteger(message.input_sequence))
                        return "input_sequence: integer expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle"))
                    if (!$util.isInteger(message.target_handle))
                        return "target_handle: integer expected";
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.target_grid, _depth + 1);
                    if (error)
                        return "target_grid." + error;
                }
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction"))
                    if (!$util.isInteger(message.direction))
                        return "direction: integer expected";
                return null;
            };

            /**
             * Gets the type url for CastIntent
             * @function getTypeUrl
             * @memberof minimir.v1.CastIntent
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            CastIntent.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.CastIntent";
            };

            return CastIntent;
        })();

        v1.CastAccepted = (function() {

            /**
             * Properties of a CastAccepted.
             * @typedef {Object} minimir.v1.CastAccepted.$Properties
             * @property {Long|null} [scene_epoch] CastAccepted scene_epoch
             * @property {Long|null} [cast_id] CastAccepted cast_id
             * @property {number|null} [skill_id] CastAccepted skill_id
             * @property {number|null} [caster_handle] CastAccepted caster_handle
             * @property {number|null} [target_handle] CastAccepted target_handle
             * @property {minimir.v1.GridPoint.$Properties|null} [origin] CastAccepted origin
             * @property {minimir.v1.GridPoint.$Properties|null} [target_grid] CastAccepted target_grid
             * @property {number|null} [direction] CastAccepted direction
             * @property {number|null} [delay_ms] CastAccepted delay_ms
             * @property {number|null} [input_sequence] CastAccepted input_sequence
             * @property {Long|null} [caster_life] CastAccepted caster_life
             * @property {Long|null} [target_life] CastAccepted target_life
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a CastAccepted.
             * @memberof minimir.v1
             * @interface ICastAccepted
             * @augments minimir.v1.CastAccepted.$Properties
             * @deprecated Use minimir.v1.CastAccepted.$Properties instead.
             */

            /**
             * Shape of a CastAccepted.
             * @typedef {minimir.v1.CastAccepted.$Properties} minimir.v1.CastAccepted.$Shape
             */

            /**
             * Constructs a new CastAccepted.
             * @memberof minimir.v1
             * @classdesc Represents a CastAccepted.
             * @constructor
             * @param {minimir.v1.CastAccepted.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var CastAccepted = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * CastAccepted scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CastAccepted cast_id.
             * @member {Long} cast_id
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.cast_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CastAccepted skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.skill_id = 0;

            /**
             * CastAccepted caster_handle.
             * @member {number} caster_handle
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.caster_handle = 0;

            /**
             * CastAccepted target_handle.
             * @member {number} target_handle
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.target_handle = 0;

            /**
             * CastAccepted origin.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} origin
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.origin = null;

            /**
             * CastAccepted target_grid.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} target_grid
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.target_grid = null;

            /**
             * CastAccepted direction.
             * @member {number} direction
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.direction = 0;

            /**
             * CastAccepted delay_ms.
             * @member {number} delay_ms
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.delay_ms = 0;

            /**
             * CastAccepted input_sequence.
             * @member {number} input_sequence
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.input_sequence = 0;

            /**
             * CastAccepted caster_life.
             * @member {Long} caster_life
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.caster_life = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CastAccepted target_life.
             * @member {Long} target_life
             * @memberof minimir.v1.CastAccepted
             * @instance
             */
            CastAccepted.prototype.target_life = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new CastAccepted instance using the specified properties.
             * @function create
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {minimir.v1.CastAccepted.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.CastAccepted} CastAccepted instance
             * @type {{
             *   (properties: minimir.v1.CastAccepted.$Shape): minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape;
             *   (properties?: minimir.v1.CastAccepted.$Properties): minimir.v1.CastAccepted;
             * }}
             */
            CastAccepted.create = function(properties) {
                return new CastAccepted(properties);
            };

            /**
             * Encodes the specified CastAccepted message. Does not implicitly {@link minimir.v1.CastAccepted.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {minimir.v1.CastAccepted.$Properties} message CastAccepted message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CastAccepted.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.cast_id != null && $Object.hasOwnProperty.call(message, "cast_id") && (typeof message.cast_id === "object" ? message.cast_id.low || message.cast_id.high : message.cast_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.cast_id);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.skill_id);
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle") && message.caster_handle !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.caster_handle);
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle") && message.target_handle !== 0)
                    writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.target_handle);
                if (message.origin != null && $Object.hasOwnProperty.call(message, "origin"))
                    $root.minimir.v1.GridPoint.encode(message.origin, writer.uint32(/* id 6, wireType 2 =*/50).fork(), _depth + 1).ldelim();
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid"))
                    $root.minimir.v1.GridPoint.encode(message.target_grid, writer.uint32(/* id 7, wireType 2 =*/58).fork(), _depth + 1).ldelim();
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction") && message.direction !== 0)
                    writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.direction);
                if (message.delay_ms != null && $Object.hasOwnProperty.call(message, "delay_ms") && message.delay_ms !== 0)
                    writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.delay_ms);
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence") && message.input_sequence !== 0)
                    writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.input_sequence);
                if (message.caster_life != null && $Object.hasOwnProperty.call(message, "caster_life") && (typeof message.caster_life === "object" ? message.caster_life.low || message.caster_life.high : message.caster_life !== 0))
                    writer.uint32(/* id 11, wireType 0 =*/88).uint64(message.caster_life);
                if (message.target_life != null && $Object.hasOwnProperty.call(message, "target_life") && (typeof message.target_life === "object" ? message.target_life.low || message.target_life.high : message.target_life !== 0))
                    writer.uint32(/* id 12, wireType 0 =*/96).uint64(message.target_life);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified CastAccepted message, length delimited. Does not implicitly {@link minimir.v1.CastAccepted.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {minimir.v1.CastAccepted.$Properties} message CastAccepted message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CastAccepted.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a CastAccepted message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape} CastAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CastAccepted.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.CastAccepted();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.cast_id = value;
                            else
                                delete message.cast_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.caster_handle = value;
                            else
                                delete message.caster_handle;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.target_handle = value;
                            else
                                delete message.target_handle;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            message.origin = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.origin);
                            continue;
                        }
                    case 7: {
                            if (wireType !== 2)
                                break;
                            message.target_grid = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.target_grid);
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.direction = value;
                            else
                                delete message.direction;
                            continue;
                        }
                    case 9: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.delay_ms = value;
                            else
                                delete message.delay_ms;
                            continue;
                        }
                    case 10: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.input_sequence = value;
                            else
                                delete message.input_sequence;
                            continue;
                        }
                    case 11: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.caster_life = value;
                            else
                                delete message.caster_life;
                            continue;
                        }
                    case 12: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.target_life = value;
                            else
                                delete message.target_life;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a CastAccepted message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.CastAccepted & minimir.v1.CastAccepted.$Shape} CastAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CastAccepted.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a CastAccepted message.
             * @function verify
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            CastAccepted.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.cast_id != null && $Object.hasOwnProperty.call(message, "cast_id"))
                    if (!$util.isInteger(message.cast_id) && !(message.cast_id && $util.isInteger(message.cast_id.low) && $util.isInteger(message.cast_id.high)))
                        return "cast_id: integer|Long expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle"))
                    if (!$util.isInteger(message.caster_handle))
                        return "caster_handle: integer expected";
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle"))
                    if (!$util.isInteger(message.target_handle))
                        return "target_handle: integer expected";
                if (message.origin != null && $Object.hasOwnProperty.call(message, "origin")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.origin, _depth + 1);
                    if (error)
                        return "origin." + error;
                }
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.target_grid, _depth + 1);
                    if (error)
                        return "target_grid." + error;
                }
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction"))
                    if (!$util.isInteger(message.direction))
                        return "direction: integer expected";
                if (message.delay_ms != null && $Object.hasOwnProperty.call(message, "delay_ms"))
                    if (!$util.isInteger(message.delay_ms))
                        return "delay_ms: integer expected";
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence"))
                    if (!$util.isInteger(message.input_sequence))
                        return "input_sequence: integer expected";
                if (message.caster_life != null && $Object.hasOwnProperty.call(message, "caster_life"))
                    if (!$util.isInteger(message.caster_life) && !(message.caster_life && $util.isInteger(message.caster_life.low) && $util.isInteger(message.caster_life.high)))
                        return "caster_life: integer|Long expected";
                if (message.target_life != null && $Object.hasOwnProperty.call(message, "target_life"))
                    if (!$util.isInteger(message.target_life) && !(message.target_life && $util.isInteger(message.target_life.low) && $util.isInteger(message.target_life.high)))
                        return "target_life: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for CastAccepted
             * @function getTypeUrl
             * @memberof minimir.v1.CastAccepted
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            CastAccepted.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.CastAccepted";
            };

            return CastAccepted;
        })();

        v1.AttackAccepted = (function() {

            /**
             * Properties of an AttackAccepted.
             * @typedef {Object} minimir.v1.AttackAccepted.$Properties
             * @property {Long|null} [scene_epoch] AttackAccepted scene_epoch
             * @property {Long|null} [event_id] AttackAccepted event_id
             * @property {number|null} [caster_handle] AttackAccepted caster_handle
             * @property {number|null} [target_handle] AttackAccepted target_handle
             * @property {minimir.v1.GridPoint.$Properties|null} [origin] AttackAccepted origin
             * @property {minimir.v1.GridPoint.$Properties|null} [target_grid] AttackAccepted target_grid
             * @property {number|null} [direction] AttackAccepted direction
             * @property {number|null} [delay_ms] AttackAccepted delay_ms
             * @property {number|null} [attack_sequence] AttackAccepted attack_sequence
             * @property {number|null} [input_sequence] AttackAccepted input_sequence
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an AttackAccepted.
             * @memberof minimir.v1
             * @interface IAttackAccepted
             * @augments minimir.v1.AttackAccepted.$Properties
             * @deprecated Use minimir.v1.AttackAccepted.$Properties instead.
             */

            /**
             * Shape of an AttackAccepted.
             * @typedef {minimir.v1.AttackAccepted.$Properties} minimir.v1.AttackAccepted.$Shape
             */

            /**
             * Constructs a new AttackAccepted.
             * @memberof minimir.v1
             * @classdesc Represents an AttackAccepted.
             * @constructor
             * @param {minimir.v1.AttackAccepted.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var AttackAccepted = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * AttackAccepted scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * AttackAccepted event_id.
             * @member {Long} event_id
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.event_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * AttackAccepted caster_handle.
             * @member {number} caster_handle
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.caster_handle = 0;

            /**
             * AttackAccepted target_handle.
             * @member {number} target_handle
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.target_handle = 0;

            /**
             * AttackAccepted origin.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} origin
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.origin = null;

            /**
             * AttackAccepted target_grid.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} target_grid
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.target_grid = null;

            /**
             * AttackAccepted direction.
             * @member {number} direction
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.direction = 0;

            /**
             * AttackAccepted delay_ms.
             * @member {number} delay_ms
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.delay_ms = 0;

            /**
             * AttackAccepted attack_sequence.
             * @member {number} attack_sequence
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.attack_sequence = 0;

            /**
             * AttackAccepted input_sequence.
             * @member {number} input_sequence
             * @memberof minimir.v1.AttackAccepted
             * @instance
             */
            AttackAccepted.prototype.input_sequence = 0;

            /**
             * Creates a new AttackAccepted instance using the specified properties.
             * @function create
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {minimir.v1.AttackAccepted.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.AttackAccepted} AttackAccepted instance
             * @type {{
             *   (properties: minimir.v1.AttackAccepted.$Shape): minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape;
             *   (properties?: minimir.v1.AttackAccepted.$Properties): minimir.v1.AttackAccepted;
             * }}
             */
            AttackAccepted.create = function(properties) {
                return new AttackAccepted(properties);
            };

            /**
             * Encodes the specified AttackAccepted message. Does not implicitly {@link minimir.v1.AttackAccepted.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {minimir.v1.AttackAccepted.$Properties} message AttackAccepted message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            AttackAccepted.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.event_id != null && $Object.hasOwnProperty.call(message, "event_id") && (typeof message.event_id === "object" ? message.event_id.low || message.event_id.high : message.event_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.event_id);
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle") && message.caster_handle !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.caster_handle);
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle") && message.target_handle !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.target_handle);
                if (message.origin != null && $Object.hasOwnProperty.call(message, "origin"))
                    $root.minimir.v1.GridPoint.encode(message.origin, writer.uint32(/* id 5, wireType 2 =*/42).fork(), _depth + 1).ldelim();
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid"))
                    $root.minimir.v1.GridPoint.encode(message.target_grid, writer.uint32(/* id 6, wireType 2 =*/50).fork(), _depth + 1).ldelim();
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction") && message.direction !== 0)
                    writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.direction);
                if (message.delay_ms != null && $Object.hasOwnProperty.call(message, "delay_ms") && message.delay_ms !== 0)
                    writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.delay_ms);
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence") && message.attack_sequence !== 0)
                    writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.attack_sequence);
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence") && message.input_sequence !== 0)
                    writer.uint32(/* id 10, wireType 0 =*/80).uint32(message.input_sequence);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified AttackAccepted message, length delimited. Does not implicitly {@link minimir.v1.AttackAccepted.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {minimir.v1.AttackAccepted.$Properties} message AttackAccepted message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            AttackAccepted.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an AttackAccepted message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape} AttackAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            AttackAccepted.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.AttackAccepted();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.event_id = value;
                            else
                                delete message.event_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.caster_handle = value;
                            else
                                delete message.caster_handle;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.target_handle = value;
                            else
                                delete message.target_handle;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 2)
                                break;
                            message.origin = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.origin);
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            message.target_grid = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.target_grid);
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.direction = value;
                            else
                                delete message.direction;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.delay_ms = value;
                            else
                                delete message.delay_ms;
                            continue;
                        }
                    case 9: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.attack_sequence = value;
                            else
                                delete message.attack_sequence;
                            continue;
                        }
                    case 10: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.input_sequence = value;
                            else
                                delete message.input_sequence;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an AttackAccepted message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.AttackAccepted & minimir.v1.AttackAccepted.$Shape} AttackAccepted
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            AttackAccepted.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an AttackAccepted message.
             * @function verify
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            AttackAccepted.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.event_id != null && $Object.hasOwnProperty.call(message, "event_id"))
                    if (!$util.isInteger(message.event_id) && !(message.event_id && $util.isInteger(message.event_id.low) && $util.isInteger(message.event_id.high)))
                        return "event_id: integer|Long expected";
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle"))
                    if (!$util.isInteger(message.caster_handle))
                        return "caster_handle: integer expected";
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle"))
                    if (!$util.isInteger(message.target_handle))
                        return "target_handle: integer expected";
                if (message.origin != null && $Object.hasOwnProperty.call(message, "origin")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.origin, _depth + 1);
                    if (error)
                        return "origin." + error;
                }
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.target_grid, _depth + 1);
                    if (error)
                        return "target_grid." + error;
                }
                if (message.direction != null && $Object.hasOwnProperty.call(message, "direction"))
                    if (!$util.isInteger(message.direction))
                        return "direction: integer expected";
                if (message.delay_ms != null && $Object.hasOwnProperty.call(message, "delay_ms"))
                    if (!$util.isInteger(message.delay_ms))
                        return "delay_ms: integer expected";
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence"))
                    if (!$util.isInteger(message.attack_sequence))
                        return "attack_sequence: integer expected";
                if (message.input_sequence != null && $Object.hasOwnProperty.call(message, "input_sequence"))
                    if (!$util.isInteger(message.input_sequence))
                        return "input_sequence: integer expected";
                return null;
            };

            /**
             * Gets the type url for AttackAccepted
             * @function getTypeUrl
             * @memberof minimir.v1.AttackAccepted
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            AttackAccepted.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.AttackAccepted";
            };

            return AttackAccepted;
        })();

        v1.CombatImpact = (function() {

            /**
             * Properties of a CombatImpact.
             * @typedef {Object} minimir.v1.CombatImpact.$Properties
             * @property {Long|null} [scene_epoch] CombatImpact scene_epoch
             * @property {Long|null} [event_id] CombatImpact event_id
             * @property {number|null} [skill_id] CombatImpact skill_id
             * @property {number|null} [caster_handle] CombatImpact caster_handle
             * @property {number|null} [target_handle] CombatImpact target_handle
             * @property {Long|null} [target_life] CombatImpact target_life
             * @property {minimir.v1.GridPoint.$Properties|null} [target_grid] CombatImpact target_grid
             * @property {Long|null} [amount] CombatImpact amount
             * @property {number|null} [outcome] CombatImpact outcome
             * @property {boolean|null} [healing] CombatImpact healing
             * @property {Long|null} [cast_id] CombatImpact cast_id
             * @property {number|null} [attack_sequence] CombatImpact attack_sequence
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a CombatImpact.
             * @memberof minimir.v1
             * @interface ICombatImpact
             * @augments minimir.v1.CombatImpact.$Properties
             * @deprecated Use minimir.v1.CombatImpact.$Properties instead.
             */

            /**
             * Shape of a CombatImpact.
             * @typedef {minimir.v1.CombatImpact.$Properties} minimir.v1.CombatImpact.$Shape
             */

            /**
             * Constructs a new CombatImpact.
             * @memberof minimir.v1
             * @classdesc Represents a CombatImpact.
             * @constructor
             * @param {minimir.v1.CombatImpact.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var CombatImpact = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * CombatImpact scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CombatImpact event_id.
             * @member {Long} event_id
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.event_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CombatImpact skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.skill_id = 0;

            /**
             * CombatImpact caster_handle.
             * @member {number} caster_handle
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.caster_handle = 0;

            /**
             * CombatImpact target_handle.
             * @member {number} target_handle
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.target_handle = 0;

            /**
             * CombatImpact target_life.
             * @member {Long} target_life
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.target_life = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CombatImpact target_grid.
             * @member {minimir.v1.GridPoint.$Properties|null|undefined} target_grid
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.target_grid = null;

            /**
             * CombatImpact amount.
             * @member {Long} amount
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.amount = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * CombatImpact outcome.
             * @member {number} outcome
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.outcome = 0;

            /**
             * CombatImpact healing.
             * @member {boolean} healing
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.healing = false;

            /**
             * CombatImpact cast_id.
             * @member {Long} cast_id
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.cast_id = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * CombatImpact attack_sequence.
             * @member {number} attack_sequence
             * @memberof minimir.v1.CombatImpact
             * @instance
             */
            CombatImpact.prototype.attack_sequence = 0;

            /**
             * Creates a new CombatImpact instance using the specified properties.
             * @function create
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {minimir.v1.CombatImpact.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.CombatImpact} CombatImpact instance
             * @type {{
             *   (properties: minimir.v1.CombatImpact.$Shape): minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape;
             *   (properties?: minimir.v1.CombatImpact.$Properties): minimir.v1.CombatImpact;
             * }}
             */
            CombatImpact.create = function(properties) {
                return new CombatImpact(properties);
            };

            /**
             * Encodes the specified CombatImpact message. Does not implicitly {@link minimir.v1.CombatImpact.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {minimir.v1.CombatImpact.$Properties} message CombatImpact message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CombatImpact.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.event_id != null && $Object.hasOwnProperty.call(message, "event_id") && (typeof message.event_id === "object" ? message.event_id.low || message.event_id.high : message.event_id !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.event_id);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.skill_id);
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle") && message.caster_handle !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.caster_handle);
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle") && message.target_handle !== 0)
                    writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.target_handle);
                if (message.target_life != null && $Object.hasOwnProperty.call(message, "target_life") && (typeof message.target_life === "object" ? message.target_life.low || message.target_life.high : message.target_life !== 0))
                    writer.uint32(/* id 6, wireType 0 =*/48).uint64(message.target_life);
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid"))
                    $root.minimir.v1.GridPoint.encode(message.target_grid, writer.uint32(/* id 7, wireType 2 =*/58).fork(), _depth + 1).ldelim();
                if (message.amount != null && $Object.hasOwnProperty.call(message, "amount") && (typeof message.amount === "object" ? message.amount.low || message.amount.high : message.amount !== 0))
                    writer.uint32(/* id 8, wireType 0 =*/64).int64(message.amount);
                if (message.outcome != null && $Object.hasOwnProperty.call(message, "outcome") && message.outcome !== 0)
                    writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.outcome);
                if (message.healing != null && $Object.hasOwnProperty.call(message, "healing") && message.healing !== false)
                    writer.uint32(/* id 10, wireType 0 =*/80).bool(message.healing);
                if (message.cast_id != null && $Object.hasOwnProperty.call(message, "cast_id") && (typeof message.cast_id === "object" ? message.cast_id.low || message.cast_id.high : message.cast_id !== 0))
                    writer.uint32(/* id 11, wireType 0 =*/88).uint64(message.cast_id);
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence") && message.attack_sequence !== 0)
                    writer.uint32(/* id 12, wireType 0 =*/96).uint32(message.attack_sequence);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified CombatImpact message, length delimited. Does not implicitly {@link minimir.v1.CombatImpact.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {minimir.v1.CombatImpact.$Properties} message CombatImpact message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            CombatImpact.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a CombatImpact message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape} CombatImpact
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CombatImpact.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.CombatImpact();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.event_id = value;
                            else
                                delete message.event_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.caster_handle = value;
                            else
                                delete message.caster_handle;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.target_handle = value;
                            else
                                delete message.target_handle;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.target_life = value;
                            else
                                delete message.target_life;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 2)
                                break;
                            message.target_grid = $root.minimir.v1.GridPoint.decode(reader, reader.size(), $undefined, _depth + 1, message.target_grid);
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.amount = value;
                            else
                                delete message.amount;
                            continue;
                        }
                    case 9: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.outcome = value;
                            else
                                delete message.outcome;
                            continue;
                        }
                    case 10: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.healing = value;
                            else
                                delete message.healing;
                            continue;
                        }
                    case 11: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.cast_id = value;
                            else
                                delete message.cast_id;
                            continue;
                        }
                    case 12: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.attack_sequence = value;
                            else
                                delete message.attack_sequence;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a CombatImpact message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.CombatImpact & minimir.v1.CombatImpact.$Shape} CombatImpact
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            CombatImpact.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a CombatImpact message.
             * @function verify
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            CombatImpact.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.event_id != null && $Object.hasOwnProperty.call(message, "event_id"))
                    if (!$util.isInteger(message.event_id) && !(message.event_id && $util.isInteger(message.event_id.low) && $util.isInteger(message.event_id.high)))
                        return "event_id: integer|Long expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.caster_handle != null && $Object.hasOwnProperty.call(message, "caster_handle"))
                    if (!$util.isInteger(message.caster_handle))
                        return "caster_handle: integer expected";
                if (message.target_handle != null && $Object.hasOwnProperty.call(message, "target_handle"))
                    if (!$util.isInteger(message.target_handle))
                        return "target_handle: integer expected";
                if (message.target_life != null && $Object.hasOwnProperty.call(message, "target_life"))
                    if (!$util.isInteger(message.target_life) && !(message.target_life && $util.isInteger(message.target_life.low) && $util.isInteger(message.target_life.high)))
                        return "target_life: integer|Long expected";
                if (message.target_grid != null && $Object.hasOwnProperty.call(message, "target_grid")) {
                    var error = $root.minimir.v1.GridPoint.verify(message.target_grid, _depth + 1);
                    if (error)
                        return "target_grid." + error;
                }
                if (message.amount != null && $Object.hasOwnProperty.call(message, "amount"))
                    if (!$util.isInteger(message.amount) && !(message.amount && $util.isInteger(message.amount.low) && $util.isInteger(message.amount.high)))
                        return "amount: integer|Long expected";
                if (message.outcome != null && $Object.hasOwnProperty.call(message, "outcome"))
                    if (!$util.isInteger(message.outcome))
                        return "outcome: integer expected";
                if (message.healing != null && $Object.hasOwnProperty.call(message, "healing"))
                    if (typeof message.healing !== "boolean")
                        return "healing: boolean expected";
                if (message.cast_id != null && $Object.hasOwnProperty.call(message, "cast_id"))
                    if (!$util.isInteger(message.cast_id) && !(message.cast_id && $util.isInteger(message.cast_id.low) && $util.isInteger(message.cast_id.high)))
                        return "cast_id: integer|Long expected";
                if (message.attack_sequence != null && $Object.hasOwnProperty.call(message, "attack_sequence"))
                    if (!$util.isInteger(message.attack_sequence))
                        return "attack_sequence: integer expected";
                return null;
            };

            /**
             * Gets the type url for CombatImpact
             * @function getTypeUrl
             * @memberof minimir.v1.CombatImpact
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            CombatImpact.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.CombatImpact";
            };

            return CombatImpact;
        })();

        v1.AutoBattleIntent = (function() {

            /**
             * Properties of an AutoBattleIntent.
             * @typedef {Object} minimir.v1.AutoBattleIntent.$Properties
             * @property {Long|null} [scene_epoch] AutoBattleIntent scene_epoch
             * @property {boolean|null} [enabled] AutoBattleIntent enabled
             * @property {number|null} [monster_count] AutoBattleIntent monster_count
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an AutoBattleIntent.
             * @memberof minimir.v1
             * @interface IAutoBattleIntent
             * @augments minimir.v1.AutoBattleIntent.$Properties
             * @deprecated Use minimir.v1.AutoBattleIntent.$Properties instead.
             */

            /**
             * Shape of an AutoBattleIntent.
             * @typedef {minimir.v1.AutoBattleIntent.$Properties} minimir.v1.AutoBattleIntent.$Shape
             */

            /**
             * Constructs a new AutoBattleIntent.
             * @memberof minimir.v1
             * @classdesc Represents an AutoBattleIntent.
             * @constructor
             * @param {minimir.v1.AutoBattleIntent.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var AutoBattleIntent = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * AutoBattleIntent scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.AutoBattleIntent
             * @instance
             */
            AutoBattleIntent.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * AutoBattleIntent enabled.
             * @member {boolean} enabled
             * @memberof minimir.v1.AutoBattleIntent
             * @instance
             */
            AutoBattleIntent.prototype.enabled = false;

            /**
             * AutoBattleIntent monster_count.
             * @member {number} monster_count
             * @memberof minimir.v1.AutoBattleIntent
             * @instance
             */
            AutoBattleIntent.prototype.monster_count = 0;

            /**
             * Creates a new AutoBattleIntent instance using the specified properties.
             * @function create
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {minimir.v1.AutoBattleIntent.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.AutoBattleIntent} AutoBattleIntent instance
             * @type {{
             *   (properties: minimir.v1.AutoBattleIntent.$Shape): minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape;
             *   (properties?: minimir.v1.AutoBattleIntent.$Properties): minimir.v1.AutoBattleIntent;
             * }}
             */
            AutoBattleIntent.create = function(properties) {
                return new AutoBattleIntent(properties);
            };

            /**
             * Encodes the specified AutoBattleIntent message. Does not implicitly {@link minimir.v1.AutoBattleIntent.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {minimir.v1.AutoBattleIntent.$Properties} message AutoBattleIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            AutoBattleIntent.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.scene_epoch);
                if (message.enabled != null && $Object.hasOwnProperty.call(message, "enabled") && message.enabled !== false)
                    writer.uint32(/* id 2, wireType 0 =*/16).bool(message.enabled);
                if (message.monster_count != null && $Object.hasOwnProperty.call(message, "monster_count") && message.monster_count !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.monster_count);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified AutoBattleIntent message, length delimited. Does not implicitly {@link minimir.v1.AutoBattleIntent.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {minimir.v1.AutoBattleIntent.$Properties} message AutoBattleIntent message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            AutoBattleIntent.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an AutoBattleIntent message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape} AutoBattleIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            AutoBattleIntent.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.AutoBattleIntent();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.enabled = value;
                            else
                                delete message.enabled;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.monster_count = value;
                            else
                                delete message.monster_count;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an AutoBattleIntent message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.AutoBattleIntent & minimir.v1.AutoBattleIntent.$Shape} AutoBattleIntent
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            AutoBattleIntent.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an AutoBattleIntent message.
             * @function verify
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            AutoBattleIntent.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.enabled != null && $Object.hasOwnProperty.call(message, "enabled"))
                    if (typeof message.enabled !== "boolean")
                        return "enabled: boolean expected";
                if (message.monster_count != null && $Object.hasOwnProperty.call(message, "monster_count"))
                    if (!$util.isInteger(message.monster_count))
                        return "monster_count: integer expected";
                return null;
            };

            /**
             * Gets the type url for AutoBattleIntent
             * @function getTypeUrl
             * @memberof minimir.v1.AutoBattleIntent
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            AutoBattleIntent.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.AutoBattleIntent";
            };

            return AutoBattleIntent;
        })();

        v1.HostingState = (function() {

            /**
             * Properties of a HostingState.
             * @typedef {Object} minimir.v1.HostingState.$Properties
             * @property {number|null} [remain_hosting_sec] HostingState remain_hosting_sec
             * @property {boolean|null} [auto_battle_enabled] HostingState auto_battle_enabled
             * @property {Long|null} [state_revision] HostingState state_revision
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a HostingState.
             * @memberof minimir.v1
             * @interface IHostingState
             * @augments minimir.v1.HostingState.$Properties
             * @deprecated Use minimir.v1.HostingState.$Properties instead.
             */

            /**
             * Shape of a HostingState.
             * @typedef {minimir.v1.HostingState.$Properties} minimir.v1.HostingState.$Shape
             */

            /**
             * Constructs a new HostingState.
             * @memberof minimir.v1
             * @classdesc Represents a HostingState.
             * @constructor
             * @param {minimir.v1.HostingState.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var HostingState = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * HostingState remain_hosting_sec.
             * @member {number} remain_hosting_sec
             * @memberof minimir.v1.HostingState
             * @instance
             */
            HostingState.prototype.remain_hosting_sec = 0;

            /**
             * HostingState auto_battle_enabled.
             * @member {boolean} auto_battle_enabled
             * @memberof minimir.v1.HostingState
             * @instance
             */
            HostingState.prototype.auto_battle_enabled = false;

            /**
             * HostingState state_revision.
             * @member {Long} state_revision
             * @memberof minimir.v1.HostingState
             * @instance
             */
            HostingState.prototype.state_revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new HostingState instance using the specified properties.
             * @function create
             * @memberof minimir.v1.HostingState
             * @static
             * @param {minimir.v1.HostingState.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.HostingState} HostingState instance
             * @type {{
             *   (properties: minimir.v1.HostingState.$Shape): minimir.v1.HostingState & minimir.v1.HostingState.$Shape;
             *   (properties?: minimir.v1.HostingState.$Properties): minimir.v1.HostingState;
             * }}
             */
            HostingState.create = function(properties) {
                return new HostingState(properties);
            };

            /**
             * Encodes the specified HostingState message. Does not implicitly {@link minimir.v1.HostingState.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.HostingState
             * @static
             * @param {minimir.v1.HostingState.$Properties} message HostingState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HostingState.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.remain_hosting_sec != null && $Object.hasOwnProperty.call(message, "remain_hosting_sec") && message.remain_hosting_sec !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.remain_hosting_sec);
                if (message.auto_battle_enabled != null && $Object.hasOwnProperty.call(message, "auto_battle_enabled") && message.auto_battle_enabled !== false)
                    writer.uint32(/* id 2, wireType 0 =*/16).bool(message.auto_battle_enabled);
                if (message.state_revision != null && $Object.hasOwnProperty.call(message, "state_revision") && (typeof message.state_revision === "object" ? message.state_revision.low || message.state_revision.high : message.state_revision !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.state_revision);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified HostingState message, length delimited. Does not implicitly {@link minimir.v1.HostingState.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.HostingState
             * @static
             * @param {minimir.v1.HostingState.$Properties} message HostingState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HostingState.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a HostingState message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.HostingState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.HostingState & minimir.v1.HostingState.$Shape} HostingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HostingState.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.HostingState();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.remain_hosting_sec = value;
                            else
                                delete message.remain_hosting_sec;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.auto_battle_enabled = value;
                            else
                                delete message.auto_battle_enabled;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.state_revision = value;
                            else
                                delete message.state_revision;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a HostingState message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.HostingState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.HostingState & minimir.v1.HostingState.$Shape} HostingState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HostingState.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a HostingState message.
             * @function verify
             * @memberof minimir.v1.HostingState
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            HostingState.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.remain_hosting_sec != null && $Object.hasOwnProperty.call(message, "remain_hosting_sec"))
                    if (!$util.isInteger(message.remain_hosting_sec))
                        return "remain_hosting_sec: integer expected";
                if (message.auto_battle_enabled != null && $Object.hasOwnProperty.call(message, "auto_battle_enabled"))
                    if (typeof message.auto_battle_enabled !== "boolean")
                        return "auto_battle_enabled: boolean expected";
                if (message.state_revision != null && $Object.hasOwnProperty.call(message, "state_revision"))
                    if (!$util.isInteger(message.state_revision) && !(message.state_revision && $util.isInteger(message.state_revision.low) && $util.isInteger(message.state_revision.high)))
                        return "state_revision: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for HostingState
             * @function getTypeUrl
             * @memberof minimir.v1.HostingState
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            HostingState.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.HostingState";
            };

            return HostingState;
        })();

        v1.DeathNotice = (function() {

            /**
             * Properties of a DeathNotice.
             * @typedef {Object} minimir.v1.DeathNotice.$Properties
             * @property {Uint8Array|null} [death_id] DeathNotice death_id
             * @property {number|null} [cause_code] DeathNotice cause_code
             * @property {number|null} [allowed_actions] DeathNotice allowed_actions
             * @property {Long|null} [inplace_price_yuanbao_units] DeathNotice inplace_price_yuanbao_units
             * @property {Long|null} [rule_version] DeathNotice rule_version
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a DeathNotice.
             * @memberof minimir.v1
             * @interface IDeathNotice
             * @augments minimir.v1.DeathNotice.$Properties
             * @deprecated Use minimir.v1.DeathNotice.$Properties instead.
             */

            /**
             * Shape of a DeathNotice.
             * @typedef {minimir.v1.DeathNotice.$Properties} minimir.v1.DeathNotice.$Shape
             */

            /**
             * Constructs a new DeathNotice.
             * @memberof minimir.v1
             * @classdesc Represents a DeathNotice.
             * @constructor
             * @param {minimir.v1.DeathNotice.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var DeathNotice = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * DeathNotice death_id.
             * @member {Uint8Array} death_id
             * @memberof minimir.v1.DeathNotice
             * @instance
             */
            DeathNotice.prototype.death_id = $util.newBuffer([]);

            /**
             * DeathNotice cause_code.
             * @member {number} cause_code
             * @memberof minimir.v1.DeathNotice
             * @instance
             */
            DeathNotice.prototype.cause_code = 0;

            /**
             * DeathNotice allowed_actions.
             * @member {number} allowed_actions
             * @memberof minimir.v1.DeathNotice
             * @instance
             */
            DeathNotice.prototype.allowed_actions = 0;

            /**
             * DeathNotice inplace_price_yuanbao_units.
             * @member {Long} inplace_price_yuanbao_units
             * @memberof minimir.v1.DeathNotice
             * @instance
             */
            DeathNotice.prototype.inplace_price_yuanbao_units = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * DeathNotice rule_version.
             * @member {Long} rule_version
             * @memberof minimir.v1.DeathNotice
             * @instance
             */
            DeathNotice.prototype.rule_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new DeathNotice instance using the specified properties.
             * @function create
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {minimir.v1.DeathNotice.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.DeathNotice} DeathNotice instance
             * @type {{
             *   (properties: minimir.v1.DeathNotice.$Shape): minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape;
             *   (properties?: minimir.v1.DeathNotice.$Properties): minimir.v1.DeathNotice;
             * }}
             */
            DeathNotice.create = function(properties) {
                return new DeathNotice(properties);
            };

            /**
             * Encodes the specified DeathNotice message. Does not implicitly {@link minimir.v1.DeathNotice.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {minimir.v1.DeathNotice.$Properties} message DeathNotice message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            DeathNotice.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.death_id != null && $Object.hasOwnProperty.call(message, "death_id") && message.death_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.death_id);
                if (message.cause_code != null && $Object.hasOwnProperty.call(message, "cause_code") && message.cause_code !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.cause_code);
                if (message.allowed_actions != null && $Object.hasOwnProperty.call(message, "allowed_actions") && message.allowed_actions !== 0)
                    writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.allowed_actions);
                if (message.inplace_price_yuanbao_units != null && $Object.hasOwnProperty.call(message, "inplace_price_yuanbao_units") && (typeof message.inplace_price_yuanbao_units === "object" ? message.inplace_price_yuanbao_units.low || message.inplace_price_yuanbao_units.high : message.inplace_price_yuanbao_units !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).int64(message.inplace_price_yuanbao_units);
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version") && (typeof message.rule_version === "object" ? message.rule_version.low || message.rule_version.high : message.rule_version !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.rule_version);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified DeathNotice message, length delimited. Does not implicitly {@link minimir.v1.DeathNotice.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {minimir.v1.DeathNotice.$Properties} message DeathNotice message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            DeathNotice.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a DeathNotice message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape} DeathNotice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            DeathNotice.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.DeathNotice();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.death_id = value;
                            else
                                delete message.death_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.cause_code = value;
                            else
                                delete message.cause_code;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.allowed_actions = value;
                            else
                                delete message.allowed_actions;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.inplace_price_yuanbao_units = value;
                            else
                                delete message.inplace_price_yuanbao_units;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.rule_version = value;
                            else
                                delete message.rule_version;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a DeathNotice message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.DeathNotice & minimir.v1.DeathNotice.$Shape} DeathNotice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            DeathNotice.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a DeathNotice message.
             * @function verify
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            DeathNotice.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.death_id != null && $Object.hasOwnProperty.call(message, "death_id"))
                    if (!(message.death_id && typeof message.death_id.length === "number" || $util.isString(message.death_id)))
                        return "death_id: buffer expected";
                if (message.cause_code != null && $Object.hasOwnProperty.call(message, "cause_code"))
                    if (!$util.isInteger(message.cause_code))
                        return "cause_code: integer expected";
                if (message.allowed_actions != null && $Object.hasOwnProperty.call(message, "allowed_actions"))
                    if (!$util.isInteger(message.allowed_actions))
                        return "allowed_actions: integer expected";
                if (message.inplace_price_yuanbao_units != null && $Object.hasOwnProperty.call(message, "inplace_price_yuanbao_units"))
                    if (!$util.isInteger(message.inplace_price_yuanbao_units) && !(message.inplace_price_yuanbao_units && $util.isInteger(message.inplace_price_yuanbao_units.low) && $util.isInteger(message.inplace_price_yuanbao_units.high)))
                        return "inplace_price_yuanbao_units: integer|Long expected";
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version"))
                    if (!$util.isInteger(message.rule_version) && !(message.rule_version && $util.isInteger(message.rule_version.low) && $util.isInteger(message.rule_version.high)))
                        return "rule_version: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for DeathNotice
             * @function getTypeUrl
             * @memberof minimir.v1.DeathNotice
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            DeathNotice.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.DeathNotice";
            };

            return DeathNotice;
        })();

        v1.ReviveChoice = (function() {

            /**
             * Properties of a ReviveChoice.
             * @typedef {Object} minimir.v1.ReviveChoice.$Properties
             * @property {Uint8Array|null} [death_id] ReviveChoice death_id
             * @property {number|null} [action] ReviveChoice action
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] ReviveChoice operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a ReviveChoice.
             * @memberof minimir.v1
             * @interface IReviveChoice
             * @augments minimir.v1.ReviveChoice.$Properties
             * @deprecated Use minimir.v1.ReviveChoice.$Properties instead.
             */

            /**
             * Shape of a ReviveChoice.
             * @typedef {minimir.v1.ReviveChoice.$Properties} minimir.v1.ReviveChoice.$Shape
             */

            /**
             * Constructs a new ReviveChoice.
             * @memberof minimir.v1
             * @classdesc Represents a ReviveChoice.
             * @constructor
             * @param {minimir.v1.ReviveChoice.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var ReviveChoice = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * ReviveChoice death_id.
             * @member {Uint8Array} death_id
             * @memberof minimir.v1.ReviveChoice
             * @instance
             */
            ReviveChoice.prototype.death_id = $util.newBuffer([]);

            /**
             * ReviveChoice action.
             * @member {number} action
             * @memberof minimir.v1.ReviveChoice
             * @instance
             */
            ReviveChoice.prototype.action = 0;

            /**
             * ReviveChoice operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.ReviveChoice
             * @instance
             */
            ReviveChoice.prototype.operation = null;

            /**
             * Creates a new ReviveChoice instance using the specified properties.
             * @function create
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {minimir.v1.ReviveChoice.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.ReviveChoice} ReviveChoice instance
             * @type {{
             *   (properties: minimir.v1.ReviveChoice.$Shape): minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape;
             *   (properties?: minimir.v1.ReviveChoice.$Properties): minimir.v1.ReviveChoice;
             * }}
             */
            ReviveChoice.create = function(properties) {
                return new ReviveChoice(properties);
            };

            /**
             * Encodes the specified ReviveChoice message. Does not implicitly {@link minimir.v1.ReviveChoice.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {minimir.v1.ReviveChoice.$Properties} message ReviveChoice message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ReviveChoice.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.death_id != null && $Object.hasOwnProperty.call(message, "death_id") && message.death_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.death_id);
                if (message.action != null && $Object.hasOwnProperty.call(message, "action") && message.action !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.action);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 3, wireType 2 =*/26).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified ReviveChoice message, length delimited. Does not implicitly {@link minimir.v1.ReviveChoice.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {minimir.v1.ReviveChoice.$Properties} message ReviveChoice message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ReviveChoice.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a ReviveChoice message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape} ReviveChoice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ReviveChoice.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.ReviveChoice();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.death_id = value;
                            else
                                delete message.death_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.action = value;
                            else
                                delete message.action;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a ReviveChoice message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.ReviveChoice & minimir.v1.ReviveChoice.$Shape} ReviveChoice
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ReviveChoice.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a ReviveChoice message.
             * @function verify
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            ReviveChoice.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.death_id != null && $Object.hasOwnProperty.call(message, "death_id"))
                    if (!(message.death_id && typeof message.death_id.length === "number" || $util.isString(message.death_id)))
                        return "death_id: buffer expected";
                if (message.action != null && $Object.hasOwnProperty.call(message, "action"))
                    if (!$util.isInteger(message.action))
                        return "action: integer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for ReviveChoice
             * @function getTypeUrl
             * @memberof minimir.v1.ReviveChoice
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            ReviveChoice.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.ReviveChoice";
            };

            return ReviveChoice;
        })();

        v1.WalletState = (function() {

            /**
             * Properties of a WalletState.
             * @typedef {Object} minimir.v1.WalletState.$Properties
             * @property {Long|null} [gold] WalletState gold
             * @property {Long|null} [bound_gold] WalletState bound_gold
             * @property {Long|null} [yuanbao_units] WalletState yuanbao_units
             * @property {Long|null} [bound_yuanbao_units] WalletState bound_yuanbao_units
             * @property {Long|null} [revision] WalletState revision
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a WalletState.
             * @memberof minimir.v1
             * @interface IWalletState
             * @augments minimir.v1.WalletState.$Properties
             * @deprecated Use minimir.v1.WalletState.$Properties instead.
             */

            /**
             * Shape of a WalletState.
             * @typedef {minimir.v1.WalletState.$Properties} minimir.v1.WalletState.$Shape
             */

            /**
             * Constructs a new WalletState.
             * @memberof minimir.v1
             * @classdesc Represents a WalletState.
             * @constructor
             * @param {minimir.v1.WalletState.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var WalletState = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * WalletState gold.
             * @member {Long} gold
             * @memberof minimir.v1.WalletState
             * @instance
             */
            WalletState.prototype.gold = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * WalletState bound_gold.
             * @member {Long} bound_gold
             * @memberof minimir.v1.WalletState
             * @instance
             */
            WalletState.prototype.bound_gold = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * WalletState yuanbao_units.
             * @member {Long} yuanbao_units
             * @memberof minimir.v1.WalletState
             * @instance
             */
            WalletState.prototype.yuanbao_units = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * WalletState bound_yuanbao_units.
             * @member {Long} bound_yuanbao_units
             * @memberof minimir.v1.WalletState
             * @instance
             */
            WalletState.prototype.bound_yuanbao_units = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * WalletState revision.
             * @member {Long} revision
             * @memberof minimir.v1.WalletState
             * @instance
             */
            WalletState.prototype.revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new WalletState instance using the specified properties.
             * @function create
             * @memberof minimir.v1.WalletState
             * @static
             * @param {minimir.v1.WalletState.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.WalletState} WalletState instance
             * @type {{
             *   (properties: minimir.v1.WalletState.$Shape): minimir.v1.WalletState & minimir.v1.WalletState.$Shape;
             *   (properties?: minimir.v1.WalletState.$Properties): minimir.v1.WalletState;
             * }}
             */
            WalletState.create = function(properties) {
                return new WalletState(properties);
            };

            /**
             * Encodes the specified WalletState message. Does not implicitly {@link minimir.v1.WalletState.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.WalletState
             * @static
             * @param {minimir.v1.WalletState.$Properties} message WalletState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            WalletState.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.gold != null && $Object.hasOwnProperty.call(message, "gold") && (typeof message.gold === "object" ? message.gold.low || message.gold.high : message.gold !== 0))
                    writer.uint32(/* id 1, wireType 0 =*/8).int64(message.gold);
                if (message.bound_gold != null && $Object.hasOwnProperty.call(message, "bound_gold") && (typeof message.bound_gold === "object" ? message.bound_gold.low || message.bound_gold.high : message.bound_gold !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).int64(message.bound_gold);
                if (message.yuanbao_units != null && $Object.hasOwnProperty.call(message, "yuanbao_units") && (typeof message.yuanbao_units === "object" ? message.yuanbao_units.low || message.yuanbao_units.high : message.yuanbao_units !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).int64(message.yuanbao_units);
                if (message.bound_yuanbao_units != null && $Object.hasOwnProperty.call(message, "bound_yuanbao_units") && (typeof message.bound_yuanbao_units === "object" ? message.bound_yuanbao_units.low || message.bound_yuanbao_units.high : message.bound_yuanbao_units !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).int64(message.bound_yuanbao_units);
                if (message.revision != null && $Object.hasOwnProperty.call(message, "revision") && (typeof message.revision === "object" ? message.revision.low || message.revision.high : message.revision !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.revision);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified WalletState message, length delimited. Does not implicitly {@link minimir.v1.WalletState.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.WalletState
             * @static
             * @param {minimir.v1.WalletState.$Properties} message WalletState message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            WalletState.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a WalletState message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.WalletState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.WalletState & minimir.v1.WalletState.$Shape} WalletState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            WalletState.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.WalletState();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.gold = value;
                            else
                                delete message.gold;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.bound_gold = value;
                            else
                                delete message.bound_gold;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.yuanbao_units = value;
                            else
                                delete message.yuanbao_units;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.bound_yuanbao_units = value;
                            else
                                delete message.bound_yuanbao_units;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.revision = value;
                            else
                                delete message.revision;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a WalletState message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.WalletState
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.WalletState & minimir.v1.WalletState.$Shape} WalletState
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            WalletState.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a WalletState message.
             * @function verify
             * @memberof minimir.v1.WalletState
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            WalletState.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.gold != null && $Object.hasOwnProperty.call(message, "gold"))
                    if (!$util.isInteger(message.gold) && !(message.gold && $util.isInteger(message.gold.low) && $util.isInteger(message.gold.high)))
                        return "gold: integer|Long expected";
                if (message.bound_gold != null && $Object.hasOwnProperty.call(message, "bound_gold"))
                    if (!$util.isInteger(message.bound_gold) && !(message.bound_gold && $util.isInteger(message.bound_gold.low) && $util.isInteger(message.bound_gold.high)))
                        return "bound_gold: integer|Long expected";
                if (message.yuanbao_units != null && $Object.hasOwnProperty.call(message, "yuanbao_units"))
                    if (!$util.isInteger(message.yuanbao_units) && !(message.yuanbao_units && $util.isInteger(message.yuanbao_units.low) && $util.isInteger(message.yuanbao_units.high)))
                        return "yuanbao_units: integer|Long expected";
                if (message.bound_yuanbao_units != null && $Object.hasOwnProperty.call(message, "bound_yuanbao_units"))
                    if (!$util.isInteger(message.bound_yuanbao_units) && !(message.bound_yuanbao_units && $util.isInteger(message.bound_yuanbao_units.low) && $util.isInteger(message.bound_yuanbao_units.high)))
                        return "bound_yuanbao_units: integer|Long expected";
                if (message.revision != null && $Object.hasOwnProperty.call(message, "revision"))
                    if (!$util.isInteger(message.revision) && !(message.revision && $util.isInteger(message.revision.low) && $util.isInteger(message.revision.high)))
                        return "revision: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for WalletState
             * @function getTypeUrl
             * @memberof minimir.v1.WalletState
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            WalletState.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.WalletState";
            };

            return WalletState;
        })();

        v1.ItemView = (function() {

            /**
             * Properties of an ItemView.
             * @typedef {Object} minimir.v1.ItemView.$Properties
             * @property {Uint8Array|null} [instance_id] ItemView instance_id
             * @property {number|null} [item_id] ItemView item_id
             * @property {Long|null} [quantity] ItemView quantity
             * @property {boolean|null} [bound] ItemView bound
             * @property {Long|null} [version] ItemView version
             * @property {string|null} [source_kind] ItemView source_kind
             * @property {string|null} [source_id] ItemView source_id
             * @property {number|null} [quality] ItemView quality
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an ItemView.
             * @memberof minimir.v1
             * @interface IItemView
             * @augments minimir.v1.ItemView.$Properties
             * @deprecated Use minimir.v1.ItemView.$Properties instead.
             */

            /**
             * Shape of an ItemView.
             * @typedef {minimir.v1.ItemView.$Properties} minimir.v1.ItemView.$Shape
             */

            /**
             * Constructs a new ItemView.
             * @memberof minimir.v1
             * @classdesc Represents an ItemView.
             * @constructor
             * @param {minimir.v1.ItemView.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var ItemView = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * ItemView instance_id.
             * @member {Uint8Array} instance_id
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.instance_id = $util.newBuffer([]);

            /**
             * ItemView item_id.
             * @member {number} item_id
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.item_id = 0;

            /**
             * ItemView quantity.
             * @member {Long} quantity
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.quantity = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * ItemView bound.
             * @member {boolean} bound
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.bound = false;

            /**
             * ItemView version.
             * @member {Long} version
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * ItemView source_kind.
             * @member {string} source_kind
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.source_kind = "";

            /**
             * ItemView source_id.
             * @member {string} source_id
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.source_id = "";

            /**
             * ItemView quality.
             * @member {number} quality
             * @memberof minimir.v1.ItemView
             * @instance
             */
            ItemView.prototype.quality = 0;

            /**
             * Creates a new ItemView instance using the specified properties.
             * @function create
             * @memberof minimir.v1.ItemView
             * @static
             * @param {minimir.v1.ItemView.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.ItemView} ItemView instance
             * @type {{
             *   (properties: minimir.v1.ItemView.$Shape): minimir.v1.ItemView & minimir.v1.ItemView.$Shape;
             *   (properties?: minimir.v1.ItemView.$Properties): minimir.v1.ItemView;
             * }}
             */
            ItemView.create = function(properties) {
                return new ItemView(properties);
            };

            /**
             * Encodes the specified ItemView message. Does not implicitly {@link minimir.v1.ItemView.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.ItemView
             * @static
             * @param {minimir.v1.ItemView.$Properties} message ItemView message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ItemView.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.instance_id != null && $Object.hasOwnProperty.call(message, "instance_id") && message.instance_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.instance_id);
                if (message.item_id != null && $Object.hasOwnProperty.call(message, "item_id") && message.item_id !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.item_id);
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity") && (typeof message.quantity === "object" ? message.quantity.low || message.quantity.high : message.quantity !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).int64(message.quantity);
                if (message.bound != null && $Object.hasOwnProperty.call(message, "bound") && message.bound !== false)
                    writer.uint32(/* id 4, wireType 0 =*/32).bool(message.bound);
                if (message.version != null && $Object.hasOwnProperty.call(message, "version") && (typeof message.version === "object" ? message.version.low || message.version.high : message.version !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.version);
                if (message.source_kind != null && $Object.hasOwnProperty.call(message, "source_kind") && message.source_kind !== "")
                    writer.uint32(/* id 6, wireType 2 =*/50).string(message.source_kind);
                if (message.source_id != null && $Object.hasOwnProperty.call(message, "source_id") && message.source_id !== "")
                    writer.uint32(/* id 7, wireType 2 =*/58).string(message.source_id);
                if (message.quality != null && $Object.hasOwnProperty.call(message, "quality") && message.quality !== 0)
                    writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.quality);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified ItemView message, length delimited. Does not implicitly {@link minimir.v1.ItemView.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.ItemView
             * @static
             * @param {minimir.v1.ItemView.$Properties} message ItemView message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ItemView.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an ItemView message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.ItemView
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.ItemView & minimir.v1.ItemView.$Shape} ItemView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ItemView.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.ItemView();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.instance_id = value;
                            else
                                delete message.instance_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.item_id = value;
                            else
                                delete message.item_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.quantity = value;
                            else
                                delete message.quantity;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.bool())
                                message.bound = value;
                            else
                                delete message.bound;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.version = value;
                            else
                                delete message.version;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.source_kind = value;
                            else
                                delete message.source_kind;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.source_id = value;
                            else
                                delete message.source_id;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.quality = value;
                            else
                                delete message.quality;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an ItemView message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.ItemView
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.ItemView & minimir.v1.ItemView.$Shape} ItemView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ItemView.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an ItemView message.
             * @function verify
             * @memberof minimir.v1.ItemView
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            ItemView.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.instance_id != null && $Object.hasOwnProperty.call(message, "instance_id"))
                    if (!(message.instance_id && typeof message.instance_id.length === "number" || $util.isString(message.instance_id)))
                        return "instance_id: buffer expected";
                if (message.item_id != null && $Object.hasOwnProperty.call(message, "item_id"))
                    if (!$util.isInteger(message.item_id))
                        return "item_id: integer expected";
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity"))
                    if (!$util.isInteger(message.quantity) && !(message.quantity && $util.isInteger(message.quantity.low) && $util.isInteger(message.quantity.high)))
                        return "quantity: integer|Long expected";
                if (message.bound != null && $Object.hasOwnProperty.call(message, "bound"))
                    if (typeof message.bound !== "boolean")
                        return "bound: boolean expected";
                if (message.version != null && $Object.hasOwnProperty.call(message, "version"))
                    if (!$util.isInteger(message.version) && !(message.version && $util.isInteger(message.version.low) && $util.isInteger(message.version.high)))
                        return "version: integer|Long expected";
                if (message.source_kind != null && $Object.hasOwnProperty.call(message, "source_kind"))
                    if (!$util.isString(message.source_kind))
                        return "source_kind: string expected";
                if (message.source_id != null && $Object.hasOwnProperty.call(message, "source_id"))
                    if (!$util.isString(message.source_id))
                        return "source_id: string expected";
                if (message.quality != null && $Object.hasOwnProperty.call(message, "quality"))
                    if (!$util.isInteger(message.quality))
                        return "quality: integer expected";
                return null;
            };

            /**
             * Gets the type url for ItemView
             * @function getTypeUrl
             * @memberof minimir.v1.ItemView
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            ItemView.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.ItemView";
            };

            return ItemView;
        })();

        v1.EquippedItem = (function() {

            /**
             * Properties of an EquippedItem.
             * @typedef {Object} minimir.v1.EquippedItem.$Properties
             * @property {number|null} [slot] EquippedItem slot
             * @property {minimir.v1.ItemView.$Properties|null} [item] EquippedItem item
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EquippedItem.
             * @memberof minimir.v1
             * @interface IEquippedItem
             * @augments minimir.v1.EquippedItem.$Properties
             * @deprecated Use minimir.v1.EquippedItem.$Properties instead.
             */

            /**
             * Shape of an EquippedItem.
             * @typedef {minimir.v1.EquippedItem.$Properties} minimir.v1.EquippedItem.$Shape
             */

            /**
             * Constructs a new EquippedItem.
             * @memberof minimir.v1
             * @classdesc Represents an EquippedItem.
             * @constructor
             * @param {minimir.v1.EquippedItem.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EquippedItem = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EquippedItem slot.
             * @member {number} slot
             * @memberof minimir.v1.EquippedItem
             * @instance
             */
            EquippedItem.prototype.slot = 0;

            /**
             * EquippedItem item.
             * @member {minimir.v1.ItemView.$Properties|null|undefined} item
             * @memberof minimir.v1.EquippedItem
             * @instance
             */
            EquippedItem.prototype.item = null;

            /**
             * Creates a new EquippedItem instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {minimir.v1.EquippedItem.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EquippedItem} EquippedItem instance
             * @type {{
             *   (properties: minimir.v1.EquippedItem.$Shape): minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape;
             *   (properties?: minimir.v1.EquippedItem.$Properties): minimir.v1.EquippedItem;
             * }}
             */
            EquippedItem.create = function(properties) {
                return new EquippedItem(properties);
            };

            /**
             * Encodes the specified EquippedItem message. Does not implicitly {@link minimir.v1.EquippedItem.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {minimir.v1.EquippedItem.$Properties} message EquippedItem message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EquippedItem.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot") && message.slot !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.slot);
                if (message.item != null && $Object.hasOwnProperty.call(message, "item"))
                    $root.minimir.v1.ItemView.encode(message.item, writer.uint32(/* id 2, wireType 2 =*/18).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EquippedItem message, length delimited. Does not implicitly {@link minimir.v1.EquippedItem.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {minimir.v1.EquippedItem.$Properties} message EquippedItem message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EquippedItem.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EquippedItem message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape} EquippedItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EquippedItem.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EquippedItem();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.slot = value;
                            else
                                delete message.slot;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            message.item = $root.minimir.v1.ItemView.decode(reader, reader.size(), $undefined, _depth + 1, message.item);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EquippedItem message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EquippedItem & minimir.v1.EquippedItem.$Shape} EquippedItem
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EquippedItem.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EquippedItem message.
             * @function verify
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EquippedItem.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot"))
                    if (!$util.isInteger(message.slot))
                        return "slot: integer expected";
                if (message.item != null && $Object.hasOwnProperty.call(message, "item")) {
                    var error = $root.minimir.v1.ItemView.verify(message.item, _depth + 1);
                    if (error)
                        return "item." + error;
                }
                return null;
            };

            /**
             * Gets the type url for EquippedItem
             * @function getTypeUrl
             * @memberof minimir.v1.EquippedItem
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EquippedItem.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EquippedItem";
            };

            return EquippedItem;
        })();

        v1.LearnedSkill = (function() {

            /**
             * Properties of a LearnedSkill.
             * @typedef {Object} minimir.v1.LearnedSkill.$Properties
             * @property {number|null} [skill_id] LearnedSkill skill_id
             * @property {number|null} [level] LearnedSkill level
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a LearnedSkill.
             * @memberof minimir.v1
             * @interface ILearnedSkill
             * @augments minimir.v1.LearnedSkill.$Properties
             * @deprecated Use minimir.v1.LearnedSkill.$Properties instead.
             */

            /**
             * Shape of a LearnedSkill.
             * @typedef {minimir.v1.LearnedSkill.$Properties} minimir.v1.LearnedSkill.$Shape
             */

            /**
             * Constructs a new LearnedSkill.
             * @memberof minimir.v1
             * @classdesc Represents a LearnedSkill.
             * @constructor
             * @param {minimir.v1.LearnedSkill.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var LearnedSkill = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * LearnedSkill skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.LearnedSkill
             * @instance
             */
            LearnedSkill.prototype.skill_id = 0;

            /**
             * LearnedSkill level.
             * @member {number} level
             * @memberof minimir.v1.LearnedSkill
             * @instance
             */
            LearnedSkill.prototype.level = 0;

            /**
             * Creates a new LearnedSkill instance using the specified properties.
             * @function create
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {minimir.v1.LearnedSkill.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.LearnedSkill} LearnedSkill instance
             * @type {{
             *   (properties: minimir.v1.LearnedSkill.$Shape): minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape;
             *   (properties?: minimir.v1.LearnedSkill.$Properties): minimir.v1.LearnedSkill;
             * }}
             */
            LearnedSkill.create = function(properties) {
                return new LearnedSkill(properties);
            };

            /**
             * Encodes the specified LearnedSkill message. Does not implicitly {@link minimir.v1.LearnedSkill.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {minimir.v1.LearnedSkill.$Properties} message LearnedSkill message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LearnedSkill.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.skill_id);
                if (message.level != null && $Object.hasOwnProperty.call(message, "level") && message.level !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.level);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified LearnedSkill message, length delimited. Does not implicitly {@link minimir.v1.LearnedSkill.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {minimir.v1.LearnedSkill.$Properties} message LearnedSkill message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LearnedSkill.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a LearnedSkill message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape} LearnedSkill
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LearnedSkill.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.LearnedSkill();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.level = value;
                            else
                                delete message.level;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a LearnedSkill message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.LearnedSkill & minimir.v1.LearnedSkill.$Shape} LearnedSkill
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LearnedSkill.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a LearnedSkill message.
             * @function verify
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            LearnedSkill.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.level != null && $Object.hasOwnProperty.call(message, "level"))
                    if (!$util.isInteger(message.level))
                        return "level: integer expected";
                return null;
            };

            /**
             * Gets the type url for LearnedSkill
             * @function getTypeUrl
             * @memberof minimir.v1.LearnedSkill
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            LearnedSkill.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.LearnedSkill";
            };

            return LearnedSkill;
        })();

        v1.InventoryResult = (function() {

            /**
             * Properties of an InventoryResult.
             * @typedef {Object} minimir.v1.InventoryResult.$Properties
             * @property {Array.<minimir.v1.ItemView.$Properties>|null} [items] InventoryResult items
             * @property {number|null} [capacity] InventoryResult capacity
             * @property {Long|null} [revision] InventoryResult revision
             * @property {Array.<minimir.v1.EquippedItem.$Properties>|null} [equipped] InventoryResult equipped
             * @property {number|null} [level] InventoryResult level
             * @property {Long|null} [experience] InventoryResult experience
             * @property {Array.<minimir.v1.LearnedSkill.$Properties>|null} [learned_skills] InventoryResult learned_skills
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an InventoryResult.
             * @memberof minimir.v1
             * @interface IInventoryResult
             * @augments minimir.v1.InventoryResult.$Properties
             * @deprecated Use minimir.v1.InventoryResult.$Properties instead.
             */

            /**
             * Shape of an InventoryResult.
             * @typedef {minimir.v1.InventoryResult.$Properties} minimir.v1.InventoryResult.$Shape
             */

            /**
             * Constructs a new InventoryResult.
             * @memberof minimir.v1
             * @classdesc Represents an InventoryResult.
             * @constructor
             * @param {minimir.v1.InventoryResult.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var InventoryResult = function (properties) {
                this.items = [];
                this.equipped = [];
                this.learned_skills = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * InventoryResult items.
             * @member {Array.<minimir.v1.ItemView.$Properties>} items
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.items = $util.emptyArray;

            /**
             * InventoryResult capacity.
             * @member {number} capacity
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.capacity = 0;

            /**
             * InventoryResult revision.
             * @member {Long} revision
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * InventoryResult equipped.
             * @member {Array.<minimir.v1.EquippedItem.$Properties>} equipped
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.equipped = $util.emptyArray;

            /**
             * InventoryResult level.
             * @member {number} level
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.level = 0;

            /**
             * InventoryResult experience.
             * @member {Long} experience
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.experience = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * InventoryResult learned_skills.
             * @member {Array.<minimir.v1.LearnedSkill.$Properties>} learned_skills
             * @memberof minimir.v1.InventoryResult
             * @instance
             */
            InventoryResult.prototype.learned_skills = $util.emptyArray;

            /**
             * Creates a new InventoryResult instance using the specified properties.
             * @function create
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {minimir.v1.InventoryResult.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.InventoryResult} InventoryResult instance
             * @type {{
             *   (properties: minimir.v1.InventoryResult.$Shape): minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape;
             *   (properties?: minimir.v1.InventoryResult.$Properties): minimir.v1.InventoryResult;
             * }}
             */
            InventoryResult.create = function(properties) {
                return new InventoryResult(properties);
            };

            /**
             * Encodes the specified InventoryResult message. Does not implicitly {@link minimir.v1.InventoryResult.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {minimir.v1.InventoryResult.$Properties} message InventoryResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            InventoryResult.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.items != null && message.items.length)
                    for (var i = 0; i < message.items.length; ++i)
                        $root.minimir.v1.ItemView.encode(message.items[i], writer.uint32(/* id 1, wireType 2 =*/10).fork(), _depth + 1).ldelim();
                if (message.capacity != null && $Object.hasOwnProperty.call(message, "capacity") && message.capacity !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.capacity);
                if (message.revision != null && $Object.hasOwnProperty.call(message, "revision") && (typeof message.revision === "object" ? message.revision.low || message.revision.high : message.revision !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.revision);
                if (message.equipped != null && message.equipped.length)
                    for (var i = 0; i < message.equipped.length; ++i)
                        $root.minimir.v1.EquippedItem.encode(message.equipped[i], writer.uint32(/* id 4, wireType 2 =*/34).fork(), _depth + 1).ldelim();
                if (message.level != null && $Object.hasOwnProperty.call(message, "level") && message.level !== 0)
                    writer.uint32(/* id 5, wireType 0 =*/40).uint32(message.level);
                if (message.experience != null && $Object.hasOwnProperty.call(message, "experience") && (typeof message.experience === "object" ? message.experience.low || message.experience.high : message.experience !== 0))
                    writer.uint32(/* id 6, wireType 0 =*/48).int64(message.experience);
                if (message.learned_skills != null && message.learned_skills.length)
                    for (var i = 0; i < message.learned_skills.length; ++i)
                        $root.minimir.v1.LearnedSkill.encode(message.learned_skills[i], writer.uint32(/* id 7, wireType 2 =*/58).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified InventoryResult message, length delimited. Does not implicitly {@link minimir.v1.InventoryResult.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {minimir.v1.InventoryResult.$Properties} message InventoryResult message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            InventoryResult.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an InventoryResult message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape} InventoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            InventoryResult.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.InventoryResult();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if (!(message.items && message.items.length))
                                message.items = [];
                            message.items.push($root.minimir.v1.ItemView.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.capacity = value;
                            else
                                delete message.capacity;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.revision = value;
                            else
                                delete message.revision;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 2)
                                break;
                            if (!(message.equipped && message.equipped.length))
                                message.equipped = [];
                            message.equipped.push($root.minimir.v1.EquippedItem.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.level = value;
                            else
                                delete message.level;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.experience = value;
                            else
                                delete message.experience;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 2)
                                break;
                            if (!(message.learned_skills && message.learned_skills.length))
                                message.learned_skills = [];
                            message.learned_skills.push($root.minimir.v1.LearnedSkill.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an InventoryResult message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.InventoryResult & minimir.v1.InventoryResult.$Shape} InventoryResult
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            InventoryResult.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an InventoryResult message.
             * @function verify
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            InventoryResult.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.items != null && $Object.hasOwnProperty.call(message, "items")) {
                    if (!$Array.isArray(message.items))
                        return "items: array expected";
                    for (var i = 0; i < message.items.length; ++i) {
                        var error = $root.minimir.v1.ItemView.verify(message.items[i], _depth + 1);
                        if (error)
                            return "items." + error;
                    }
                }
                if (message.capacity != null && $Object.hasOwnProperty.call(message, "capacity"))
                    if (!$util.isInteger(message.capacity))
                        return "capacity: integer expected";
                if (message.revision != null && $Object.hasOwnProperty.call(message, "revision"))
                    if (!$util.isInteger(message.revision) && !(message.revision && $util.isInteger(message.revision.low) && $util.isInteger(message.revision.high)))
                        return "revision: integer|Long expected";
                if (message.equipped != null && $Object.hasOwnProperty.call(message, "equipped")) {
                    if (!$Array.isArray(message.equipped))
                        return "equipped: array expected";
                    for (var i = 0; i < message.equipped.length; ++i) {
                        var error = $root.minimir.v1.EquippedItem.verify(message.equipped[i], _depth + 1);
                        if (error)
                            return "equipped." + error;
                    }
                }
                if (message.level != null && $Object.hasOwnProperty.call(message, "level"))
                    if (!$util.isInteger(message.level))
                        return "level: integer expected";
                if (message.experience != null && $Object.hasOwnProperty.call(message, "experience"))
                    if (!$util.isInteger(message.experience) && !(message.experience && $util.isInteger(message.experience.low) && $util.isInteger(message.experience.high)))
                        return "experience: integer|Long expected";
                if (message.learned_skills != null && $Object.hasOwnProperty.call(message, "learned_skills")) {
                    if (!$Array.isArray(message.learned_skills))
                        return "learned_skills: array expected";
                    for (var i = 0; i < message.learned_skills.length; ++i) {
                        var error = $root.minimir.v1.LearnedSkill.verify(message.learned_skills[i], _depth + 1);
                        if (error)
                            return "learned_skills." + error;
                    }
                }
                return null;
            };

            /**
             * Gets the type url for InventoryResult
             * @function getTypeUrl
             * @memberof minimir.v1.InventoryResult
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            InventoryResult.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.InventoryResult";
            };

            return InventoryResult;
        })();

        v1.EquipRequest = (function() {

            /**
             * Properties of an EquipRequest.
             * @typedef {Object} minimir.v1.EquipRequest.$Properties
             * @property {Uint8Array|null} [item_instance_id] EquipRequest item_instance_id
             * @property {number|null} [slot] EquipRequest slot
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] EquipRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an EquipRequest.
             * @memberof minimir.v1
             * @interface IEquipRequest
             * @augments minimir.v1.EquipRequest.$Properties
             * @deprecated Use minimir.v1.EquipRequest.$Properties instead.
             */

            /**
             * Shape of an EquipRequest.
             * @typedef {minimir.v1.EquipRequest.$Properties} minimir.v1.EquipRequest.$Shape
             */

            /**
             * Constructs a new EquipRequest.
             * @memberof minimir.v1
             * @classdesc Represents an EquipRequest.
             * @constructor
             * @param {minimir.v1.EquipRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var EquipRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * EquipRequest item_instance_id.
             * @member {Uint8Array} item_instance_id
             * @memberof minimir.v1.EquipRequest
             * @instance
             */
            EquipRequest.prototype.item_instance_id = $util.newBuffer([]);

            /**
             * EquipRequest slot.
             * @member {number} slot
             * @memberof minimir.v1.EquipRequest
             * @instance
             */
            EquipRequest.prototype.slot = 0;

            /**
             * EquipRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.EquipRequest
             * @instance
             */
            EquipRequest.prototype.operation = null;

            /**
             * Creates a new EquipRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {minimir.v1.EquipRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.EquipRequest} EquipRequest instance
             * @type {{
             *   (properties: minimir.v1.EquipRequest.$Shape): minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape;
             *   (properties?: minimir.v1.EquipRequest.$Properties): minimir.v1.EquipRequest;
             * }}
             */
            EquipRequest.create = function(properties) {
                return new EquipRequest(properties);
            };

            /**
             * Encodes the specified EquipRequest message. Does not implicitly {@link minimir.v1.EquipRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {minimir.v1.EquipRequest.$Properties} message EquipRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EquipRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id") && message.item_instance_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.item_instance_id);
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot") && message.slot !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.slot);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 3, wireType 2 =*/26).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified EquipRequest message, length delimited. Does not implicitly {@link minimir.v1.EquipRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {minimir.v1.EquipRequest.$Properties} message EquipRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            EquipRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an EquipRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape} EquipRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EquipRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.EquipRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.item_instance_id = value;
                            else
                                delete message.item_instance_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.slot = value;
                            else
                                delete message.slot;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an EquipRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.EquipRequest & minimir.v1.EquipRequest.$Shape} EquipRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            EquipRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an EquipRequest message.
             * @function verify
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            EquipRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id"))
                    if (!(message.item_instance_id && typeof message.item_instance_id.length === "number" || $util.isString(message.item_instance_id)))
                        return "item_instance_id: buffer expected";
                if (message.slot != null && $Object.hasOwnProperty.call(message, "slot"))
                    if (!$util.isInteger(message.slot))
                        return "slot: integer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for EquipRequest
             * @function getTypeUrl
             * @memberof minimir.v1.EquipRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            EquipRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.EquipRequest";
            };

            return EquipRequest;
        })();

        v1.RecycleRequest = (function() {

            /**
             * Properties of a RecycleRequest.
             * @typedef {Object} minimir.v1.RecycleRequest.$Properties
             * @property {Array.<Uint8Array>|null} [item_instance_ids] RecycleRequest item_instance_ids
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] RecycleRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a RecycleRequest.
             * @memberof minimir.v1
             * @interface IRecycleRequest
             * @augments minimir.v1.RecycleRequest.$Properties
             * @deprecated Use minimir.v1.RecycleRequest.$Properties instead.
             */

            /**
             * Shape of a RecycleRequest.
             * @typedef {minimir.v1.RecycleRequest.$Properties} minimir.v1.RecycleRequest.$Shape
             */

            /**
             * Constructs a new RecycleRequest.
             * @memberof minimir.v1
             * @classdesc Represents a RecycleRequest.
             * @constructor
             * @param {minimir.v1.RecycleRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var RecycleRequest = function (properties) {
                this.item_instance_ids = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * RecycleRequest item_instance_ids.
             * @member {Array.<Uint8Array>} item_instance_ids
             * @memberof minimir.v1.RecycleRequest
             * @instance
             */
            RecycleRequest.prototype.item_instance_ids = $util.emptyArray;

            /**
             * RecycleRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.RecycleRequest
             * @instance
             */
            RecycleRequest.prototype.operation = null;

            /**
             * Creates a new RecycleRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {minimir.v1.RecycleRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.RecycleRequest} RecycleRequest instance
             * @type {{
             *   (properties: minimir.v1.RecycleRequest.$Shape): minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape;
             *   (properties?: minimir.v1.RecycleRequest.$Properties): minimir.v1.RecycleRequest;
             * }}
             */
            RecycleRequest.create = function(properties) {
                return new RecycleRequest(properties);
            };

            /**
             * Encodes the specified RecycleRequest message. Does not implicitly {@link minimir.v1.RecycleRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {minimir.v1.RecycleRequest.$Properties} message RecycleRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RecycleRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.item_instance_ids != null && message.item_instance_ids.length)
                    for (var i = 0; i < message.item_instance_ids.length; ++i)
                        writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.item_instance_ids[i]);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 2, wireType 2 =*/18).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified RecycleRequest message, length delimited. Does not implicitly {@link minimir.v1.RecycleRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {minimir.v1.RecycleRequest.$Properties} message RecycleRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            RecycleRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a RecycleRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape} RecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RecycleRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.RecycleRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if (!(message.item_instance_ids && message.item_instance_ids.length))
                                message.item_instance_ids = [];
                            message.item_instance_ids.push(reader.bytes());
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a RecycleRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.RecycleRequest & minimir.v1.RecycleRequest.$Shape} RecycleRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            RecycleRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a RecycleRequest message.
             * @function verify
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            RecycleRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.item_instance_ids != null && $Object.hasOwnProperty.call(message, "item_instance_ids")) {
                    if (!$Array.isArray(message.item_instance_ids))
                        return "item_instance_ids: array expected";
                    for (var i = 0; i < message.item_instance_ids.length; ++i)
                        if (!(message.item_instance_ids[i] && typeof message.item_instance_ids[i].length === "number" || $util.isString(message.item_instance_ids[i])))
                            return "item_instance_ids: buffer[] expected";
                }
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for RecycleRequest
             * @function getTypeUrl
             * @memberof minimir.v1.RecycleRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            RecycleRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.RecycleRequest";
            };

            return RecycleRequest;
        })();

        v1.UseItemRequest = (function() {

            /**
             * Properties of a UseItemRequest.
             * @typedef {Object} minimir.v1.UseItemRequest.$Properties
             * @property {Uint8Array|null} [item_instance_id] UseItemRequest item_instance_id
             * @property {number|null} [item_id] UseItemRequest item_id
             * @property {Long|null} [scene_epoch] UseItemRequest scene_epoch
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] UseItemRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a UseItemRequest.
             * @memberof minimir.v1
             * @interface IUseItemRequest
             * @augments minimir.v1.UseItemRequest.$Properties
             * @deprecated Use minimir.v1.UseItemRequest.$Properties instead.
             */

            /**
             * Shape of a UseItemRequest.
             * @typedef {minimir.v1.UseItemRequest.$Properties} minimir.v1.UseItemRequest.$Shape
             */

            /**
             * Constructs a new UseItemRequest.
             * @memberof minimir.v1
             * @classdesc Represents a UseItemRequest.
             * @constructor
             * @param {minimir.v1.UseItemRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var UseItemRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * UseItemRequest item_instance_id.
             * @member {Uint8Array} item_instance_id
             * @memberof minimir.v1.UseItemRequest
             * @instance
             */
            UseItemRequest.prototype.item_instance_id = $util.newBuffer([]);

            /**
             * UseItemRequest item_id.
             * @member {number} item_id
             * @memberof minimir.v1.UseItemRequest
             * @instance
             */
            UseItemRequest.prototype.item_id = 0;

            /**
             * UseItemRequest scene_epoch.
             * @member {Long} scene_epoch
             * @memberof minimir.v1.UseItemRequest
             * @instance
             */
            UseItemRequest.prototype.scene_epoch = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * UseItemRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.UseItemRequest
             * @instance
             */
            UseItemRequest.prototype.operation = null;

            /**
             * Creates a new UseItemRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {minimir.v1.UseItemRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.UseItemRequest} UseItemRequest instance
             * @type {{
             *   (properties: minimir.v1.UseItemRequest.$Shape): minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape;
             *   (properties?: minimir.v1.UseItemRequest.$Properties): minimir.v1.UseItemRequest;
             * }}
             */
            UseItemRequest.create = function(properties) {
                return new UseItemRequest(properties);
            };

            /**
             * Encodes the specified UseItemRequest message. Does not implicitly {@link minimir.v1.UseItemRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {minimir.v1.UseItemRequest.$Properties} message UseItemRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            UseItemRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id") && message.item_instance_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.item_instance_id);
                if (message.item_id != null && $Object.hasOwnProperty.call(message, "item_id") && message.item_id !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.item_id);
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch") && (typeof message.scene_epoch === "object" ? message.scene_epoch.low || message.scene_epoch.high : message.scene_epoch !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.scene_epoch);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 4, wireType 2 =*/34).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified UseItemRequest message, length delimited. Does not implicitly {@link minimir.v1.UseItemRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {minimir.v1.UseItemRequest.$Properties} message UseItemRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            UseItemRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a UseItemRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape} UseItemRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            UseItemRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.UseItemRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.item_instance_id = value;
                            else
                                delete message.item_instance_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.item_id = value;
                            else
                                delete message.item_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.scene_epoch = value;
                            else
                                delete message.scene_epoch;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a UseItemRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.UseItemRequest & minimir.v1.UseItemRequest.$Shape} UseItemRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            UseItemRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a UseItemRequest message.
             * @function verify
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            UseItemRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id"))
                    if (!(message.item_instance_id && typeof message.item_instance_id.length === "number" || $util.isString(message.item_instance_id)))
                        return "item_instance_id: buffer expected";
                if (message.item_id != null && $Object.hasOwnProperty.call(message, "item_id"))
                    if (!$util.isInteger(message.item_id))
                        return "item_id: integer expected";
                if (message.scene_epoch != null && $Object.hasOwnProperty.call(message, "scene_epoch"))
                    if (!$util.isInteger(message.scene_epoch) && !(message.scene_epoch && $util.isInteger(message.scene_epoch.low) && $util.isInteger(message.scene_epoch.high)))
                        return "scene_epoch: integer|Long expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for UseItemRequest
             * @function getTypeUrl
             * @memberof minimir.v1.UseItemRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            UseItemRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.UseItemRequest";
            };

            return UseItemRequest;
        })();

        v1.LearnSkillRequest = (function() {

            /**
             * Properties of a LearnSkillRequest.
             * @typedef {Object} minimir.v1.LearnSkillRequest.$Properties
             * @property {Uint8Array|null} [item_instance_id] LearnSkillRequest item_instance_id
             * @property {number|null} [skill_id] LearnSkillRequest skill_id
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] LearnSkillRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a LearnSkillRequest.
             * @memberof minimir.v1
             * @interface ILearnSkillRequest
             * @augments minimir.v1.LearnSkillRequest.$Properties
             * @deprecated Use minimir.v1.LearnSkillRequest.$Properties instead.
             */

            /**
             * Shape of a LearnSkillRequest.
             * @typedef {minimir.v1.LearnSkillRequest.$Properties} minimir.v1.LearnSkillRequest.$Shape
             */

            /**
             * Constructs a new LearnSkillRequest.
             * @memberof minimir.v1
             * @classdesc Represents a LearnSkillRequest.
             * @constructor
             * @param {minimir.v1.LearnSkillRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var LearnSkillRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * LearnSkillRequest item_instance_id.
             * @member {Uint8Array} item_instance_id
             * @memberof minimir.v1.LearnSkillRequest
             * @instance
             */
            LearnSkillRequest.prototype.item_instance_id = $util.newBuffer([]);

            /**
             * LearnSkillRequest skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.LearnSkillRequest
             * @instance
             */
            LearnSkillRequest.prototype.skill_id = 0;

            /**
             * LearnSkillRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.LearnSkillRequest
             * @instance
             */
            LearnSkillRequest.prototype.operation = null;

            /**
             * Creates a new LearnSkillRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {minimir.v1.LearnSkillRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.LearnSkillRequest} LearnSkillRequest instance
             * @type {{
             *   (properties: minimir.v1.LearnSkillRequest.$Shape): minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape;
             *   (properties?: minimir.v1.LearnSkillRequest.$Properties): minimir.v1.LearnSkillRequest;
             * }}
             */
            LearnSkillRequest.create = function(properties) {
                return new LearnSkillRequest(properties);
            };

            /**
             * Encodes the specified LearnSkillRequest message. Does not implicitly {@link minimir.v1.LearnSkillRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {minimir.v1.LearnSkillRequest.$Properties} message LearnSkillRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LearnSkillRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id") && message.item_instance_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.item_instance_id);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.skill_id);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 3, wireType 2 =*/26).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified LearnSkillRequest message, length delimited. Does not implicitly {@link minimir.v1.LearnSkillRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {minimir.v1.LearnSkillRequest.$Properties} message LearnSkillRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            LearnSkillRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a LearnSkillRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape} LearnSkillRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LearnSkillRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.LearnSkillRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.item_instance_id = value;
                            else
                                delete message.item_instance_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a LearnSkillRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.LearnSkillRequest & minimir.v1.LearnSkillRequest.$Shape} LearnSkillRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            LearnSkillRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a LearnSkillRequest message.
             * @function verify
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            LearnSkillRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.item_instance_id != null && $Object.hasOwnProperty.call(message, "item_instance_id"))
                    if (!(message.item_instance_id && typeof message.item_instance_id.length === "number" || $util.isString(message.item_instance_id)))
                        return "item_instance_id: buffer expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for LearnSkillRequest
             * @function getTypeUrl
             * @memberof minimir.v1.LearnSkillRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            LearnSkillRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.LearnSkillRequest";
            };

            return LearnSkillRequest;
        })();

        v1.ItemConsumption = (function() {

            /**
             * Properties of an ItemConsumption.
             * @typedef {Object} minimir.v1.ItemConsumption.$Properties
             * @property {Uint8Array|null} [instance_id] ItemConsumption instance_id
             * @property {Long|null} [version] ItemConsumption version
             * @property {Long|null} [quantity] ItemConsumption quantity
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of an ItemConsumption.
             * @memberof minimir.v1
             * @interface IItemConsumption
             * @augments minimir.v1.ItemConsumption.$Properties
             * @deprecated Use minimir.v1.ItemConsumption.$Properties instead.
             */

            /**
             * Shape of an ItemConsumption.
             * @typedef {minimir.v1.ItemConsumption.$Properties} minimir.v1.ItemConsumption.$Shape
             */

            /**
             * Constructs a new ItemConsumption.
             * @memberof minimir.v1
             * @classdesc Represents an ItemConsumption.
             * @constructor
             * @param {minimir.v1.ItemConsumption.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var ItemConsumption = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * ItemConsumption instance_id.
             * @member {Uint8Array} instance_id
             * @memberof minimir.v1.ItemConsumption
             * @instance
             */
            ItemConsumption.prototype.instance_id = $util.newBuffer([]);

            /**
             * ItemConsumption version.
             * @member {Long} version
             * @memberof minimir.v1.ItemConsumption
             * @instance
             */
            ItemConsumption.prototype.version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * ItemConsumption quantity.
             * @member {Long} quantity
             * @memberof minimir.v1.ItemConsumption
             * @instance
             */
            ItemConsumption.prototype.quantity = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * Creates a new ItemConsumption instance using the specified properties.
             * @function create
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {minimir.v1.ItemConsumption.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.ItemConsumption} ItemConsumption instance
             * @type {{
             *   (properties: minimir.v1.ItemConsumption.$Shape): minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape;
             *   (properties?: minimir.v1.ItemConsumption.$Properties): minimir.v1.ItemConsumption;
             * }}
             */
            ItemConsumption.create = function(properties) {
                return new ItemConsumption(properties);
            };

            /**
             * Encodes the specified ItemConsumption message. Does not implicitly {@link minimir.v1.ItemConsumption.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {minimir.v1.ItemConsumption.$Properties} message ItemConsumption message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ItemConsumption.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.instance_id != null && $Object.hasOwnProperty.call(message, "instance_id") && message.instance_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.instance_id);
                if (message.version != null && $Object.hasOwnProperty.call(message, "version") && (typeof message.version === "object" ? message.version.low || message.version.high : message.version !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.version);
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity") && (typeof message.quantity === "object" ? message.quantity.low || message.quantity.high : message.quantity !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).int64(message.quantity);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified ItemConsumption message, length delimited. Does not implicitly {@link minimir.v1.ItemConsumption.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {minimir.v1.ItemConsumption.$Properties} message ItemConsumption message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ItemConsumption.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes an ItemConsumption message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape} ItemConsumption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ItemConsumption.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.ItemConsumption();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.instance_id = value;
                            else
                                delete message.instance_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.version = value;
                            else
                                delete message.version;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.quantity = value;
                            else
                                delete message.quantity;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes an ItemConsumption message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.ItemConsumption & minimir.v1.ItemConsumption.$Shape} ItemConsumption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ItemConsumption.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies an ItemConsumption message.
             * @function verify
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            ItemConsumption.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.instance_id != null && $Object.hasOwnProperty.call(message, "instance_id"))
                    if (!(message.instance_id && typeof message.instance_id.length === "number" || $util.isString(message.instance_id)))
                        return "instance_id: buffer expected";
                if (message.version != null && $Object.hasOwnProperty.call(message, "version"))
                    if (!$util.isInteger(message.version) && !(message.version && $util.isInteger(message.version.low) && $util.isInteger(message.version.high)))
                        return "version: integer|Long expected";
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity"))
                    if (!$util.isInteger(message.quantity) && !(message.quantity && $util.isInteger(message.quantity.low) && $util.isInteger(message.quantity.high)))
                        return "quantity: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for ItemConsumption
             * @function getTypeUrl
             * @memberof minimir.v1.ItemConsumption
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            ItemConsumption.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.ItemConsumption";
            };

            return ItemConsumption;
        })();

        v1.SkillAssetPreviewRequest = (function() {

            /**
             * Properties of a SkillAssetPreviewRequest.
             * @typedef {Object} minimir.v1.SkillAssetPreviewRequest.$Properties
             * @property {number|null} [kind] SkillAssetPreviewRequest kind
             * @property {number|null} [offer_id] SkillAssetPreviewRequest offer_id
             * @property {Long|null} [quantity] SkillAssetPreviewRequest quantity
             * @property {number|null} [skill_id] SkillAssetPreviewRequest skill_id
             * @property {Array.<minimir.v1.ItemConsumption.$Properties>|null} [materials] SkillAssetPreviewRequest materials
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SkillAssetPreviewRequest.
             * @memberof minimir.v1
             * @interface ISkillAssetPreviewRequest
             * @augments minimir.v1.SkillAssetPreviewRequest.$Properties
             * @deprecated Use minimir.v1.SkillAssetPreviewRequest.$Properties instead.
             */

            /**
             * Shape of a SkillAssetPreviewRequest.
             * @typedef {minimir.v1.SkillAssetPreviewRequest.$Properties} minimir.v1.SkillAssetPreviewRequest.$Shape
             */

            /**
             * Constructs a new SkillAssetPreviewRequest.
             * @memberof minimir.v1
             * @classdesc Represents a SkillAssetPreviewRequest.
             * @constructor
             * @param {minimir.v1.SkillAssetPreviewRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SkillAssetPreviewRequest = function (properties) {
                this.materials = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SkillAssetPreviewRequest kind.
             * @member {number} kind
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @instance
             */
            SkillAssetPreviewRequest.prototype.kind = 0;

            /**
             * SkillAssetPreviewRequest offer_id.
             * @member {number} offer_id
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @instance
             */
            SkillAssetPreviewRequest.prototype.offer_id = 0;

            /**
             * SkillAssetPreviewRequest quantity.
             * @member {Long} quantity
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @instance
             */
            SkillAssetPreviewRequest.prototype.quantity = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * SkillAssetPreviewRequest skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @instance
             */
            SkillAssetPreviewRequest.prototype.skill_id = 0;

            /**
             * SkillAssetPreviewRequest materials.
             * @member {Array.<minimir.v1.ItemConsumption.$Properties>} materials
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @instance
             */
            SkillAssetPreviewRequest.prototype.materials = $util.emptyArray;

            /**
             * Creates a new SkillAssetPreviewRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {minimir.v1.SkillAssetPreviewRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SkillAssetPreviewRequest} SkillAssetPreviewRequest instance
             * @type {{
             *   (properties: minimir.v1.SkillAssetPreviewRequest.$Shape): minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape;
             *   (properties?: minimir.v1.SkillAssetPreviewRequest.$Properties): minimir.v1.SkillAssetPreviewRequest;
             * }}
             */
            SkillAssetPreviewRequest.create = function(properties) {
                return new SkillAssetPreviewRequest(properties);
            };

            /**
             * Encodes the specified SkillAssetPreviewRequest message. Does not implicitly {@link minimir.v1.SkillAssetPreviewRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {minimir.v1.SkillAssetPreviewRequest.$Properties} message SkillAssetPreviewRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetPreviewRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.kind != null && $Object.hasOwnProperty.call(message, "kind") && message.kind !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.kind);
                if (message.offer_id != null && $Object.hasOwnProperty.call(message, "offer_id") && message.offer_id !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.offer_id);
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity") && (typeof message.quantity === "object" ? message.quantity.low || message.quantity.high : message.quantity !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).int64(message.quantity);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 4, wireType 0 =*/32).uint32(message.skill_id);
                if (message.materials != null && message.materials.length)
                    for (var i = 0; i < message.materials.length; ++i)
                        $root.minimir.v1.ItemConsumption.encode(message.materials[i], writer.uint32(/* id 5, wireType 2 =*/42).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SkillAssetPreviewRequest message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetPreviewRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {minimir.v1.SkillAssetPreviewRequest.$Properties} message SkillAssetPreviewRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetPreviewRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SkillAssetPreviewRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape} SkillAssetPreviewRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetPreviewRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SkillAssetPreviewRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.kind = value;
                            else
                                delete message.kind;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.offer_id = value;
                            else
                                delete message.offer_id;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.quantity = value;
                            else
                                delete message.quantity;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 2)
                                break;
                            if (!(message.materials && message.materials.length))
                                message.materials = [];
                            message.materials.push($root.minimir.v1.ItemConsumption.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SkillAssetPreviewRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetPreviewRequest & minimir.v1.SkillAssetPreviewRequest.$Shape} SkillAssetPreviewRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetPreviewRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SkillAssetPreviewRequest message.
             * @function verify
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SkillAssetPreviewRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.kind != null && $Object.hasOwnProperty.call(message, "kind"))
                    if (!$util.isInteger(message.kind))
                        return "kind: integer expected";
                if (message.offer_id != null && $Object.hasOwnProperty.call(message, "offer_id"))
                    if (!$util.isInteger(message.offer_id))
                        return "offer_id: integer expected";
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity"))
                    if (!$util.isInteger(message.quantity) && !(message.quantity && $util.isInteger(message.quantity.low) && $util.isInteger(message.quantity.high)))
                        return "quantity: integer|Long expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.materials != null && $Object.hasOwnProperty.call(message, "materials")) {
                    if (!$Array.isArray(message.materials))
                        return "materials: array expected";
                    for (var i = 0; i < message.materials.length; ++i) {
                        var error = $root.minimir.v1.ItemConsumption.verify(message.materials[i], _depth + 1);
                        if (error)
                            return "materials." + error;
                    }
                }
                return null;
            };

            /**
             * Gets the type url for SkillAssetPreviewRequest
             * @function getTypeUrl
             * @memberof minimir.v1.SkillAssetPreviewRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SkillAssetPreviewRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SkillAssetPreviewRequest";
            };

            return SkillAssetPreviewRequest;
        })();

        v1.SkillMaterialView = (function() {

            /**
             * Properties of a SkillMaterialView.
             * @typedef {Object} minimir.v1.SkillMaterialView.$Properties
             * @property {minimir.v1.ItemView.$Properties|null} [item] SkillMaterialView item
             * @property {Long|null} [consume_quantity] SkillMaterialView consume_quantity
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SkillMaterialView.
             * @memberof minimir.v1
             * @interface ISkillMaterialView
             * @augments minimir.v1.SkillMaterialView.$Properties
             * @deprecated Use minimir.v1.SkillMaterialView.$Properties instead.
             */

            /**
             * Shape of a SkillMaterialView.
             * @typedef {minimir.v1.SkillMaterialView.$Properties} minimir.v1.SkillMaterialView.$Shape
             */

            /**
             * Constructs a new SkillMaterialView.
             * @memberof minimir.v1
             * @classdesc Represents a SkillMaterialView.
             * @constructor
             * @param {minimir.v1.SkillMaterialView.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SkillMaterialView = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SkillMaterialView item.
             * @member {minimir.v1.ItemView.$Properties|null|undefined} item
             * @memberof minimir.v1.SkillMaterialView
             * @instance
             */
            SkillMaterialView.prototype.item = null;

            /**
             * SkillMaterialView consume_quantity.
             * @member {Long} consume_quantity
             * @memberof minimir.v1.SkillMaterialView
             * @instance
             */
            SkillMaterialView.prototype.consume_quantity = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * Creates a new SkillMaterialView instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {minimir.v1.SkillMaterialView.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SkillMaterialView} SkillMaterialView instance
             * @type {{
             *   (properties: minimir.v1.SkillMaterialView.$Shape): minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape;
             *   (properties?: minimir.v1.SkillMaterialView.$Properties): minimir.v1.SkillMaterialView;
             * }}
             */
            SkillMaterialView.create = function(properties) {
                return new SkillMaterialView(properties);
            };

            /**
             * Encodes the specified SkillMaterialView message. Does not implicitly {@link minimir.v1.SkillMaterialView.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {minimir.v1.SkillMaterialView.$Properties} message SkillMaterialView message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillMaterialView.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.item != null && $Object.hasOwnProperty.call(message, "item"))
                    $root.minimir.v1.ItemView.encode(message.item, writer.uint32(/* id 1, wireType 2 =*/10).fork(), _depth + 1).ldelim();
                if (message.consume_quantity != null && $Object.hasOwnProperty.call(message, "consume_quantity") && (typeof message.consume_quantity === "object" ? message.consume_quantity.low || message.consume_quantity.high : message.consume_quantity !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).int64(message.consume_quantity);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SkillMaterialView message, length delimited. Does not implicitly {@link minimir.v1.SkillMaterialView.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {minimir.v1.SkillMaterialView.$Properties} message SkillMaterialView message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillMaterialView.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SkillMaterialView message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape} SkillMaterialView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillMaterialView.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SkillMaterialView();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            message.item = $root.minimir.v1.ItemView.decode(reader, reader.size(), $undefined, _depth + 1, message.item);
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.consume_quantity = value;
                            else
                                delete message.consume_quantity;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SkillMaterialView message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillMaterialView & minimir.v1.SkillMaterialView.$Shape} SkillMaterialView
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillMaterialView.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SkillMaterialView message.
             * @function verify
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SkillMaterialView.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.item != null && $Object.hasOwnProperty.call(message, "item")) {
                    var error = $root.minimir.v1.ItemView.verify(message.item, _depth + 1);
                    if (error)
                        return "item." + error;
                }
                if (message.consume_quantity != null && $Object.hasOwnProperty.call(message, "consume_quantity"))
                    if (!$util.isInteger(message.consume_quantity) && !(message.consume_quantity && $util.isInteger(message.consume_quantity.low) && $util.isInteger(message.consume_quantity.high)))
                        return "consume_quantity: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for SkillMaterialView
             * @function getTypeUrl
             * @memberof minimir.v1.SkillMaterialView
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SkillMaterialView.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SkillMaterialView";
            };

            return SkillMaterialView;
        })();

        v1.SkillNumericComparison = (function() {

            /**
             * Properties of a SkillNumericComparison.
             * @typedef {Object} minimir.v1.SkillNumericComparison.$Properties
             * @property {string|null} [key] SkillNumericComparison key
             * @property {Long|null} [before_min] SkillNumericComparison before_min
             * @property {Long|null} [before_max] SkillNumericComparison before_max
             * @property {Long|null} [after_min] SkillNumericComparison after_min
             * @property {Long|null} [after_max] SkillNumericComparison after_max
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SkillNumericComparison.
             * @memberof minimir.v1
             * @interface ISkillNumericComparison
             * @augments minimir.v1.SkillNumericComparison.$Properties
             * @deprecated Use minimir.v1.SkillNumericComparison.$Properties instead.
             */

            /**
             * Shape of a SkillNumericComparison.
             * @typedef {minimir.v1.SkillNumericComparison.$Properties} minimir.v1.SkillNumericComparison.$Shape
             */

            /**
             * Constructs a new SkillNumericComparison.
             * @memberof minimir.v1
             * @classdesc Represents a SkillNumericComparison.
             * @constructor
             * @param {minimir.v1.SkillNumericComparison.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SkillNumericComparison = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SkillNumericComparison key.
             * @member {string} key
             * @memberof minimir.v1.SkillNumericComparison
             * @instance
             */
            SkillNumericComparison.prototype.key = "";

            /**
             * SkillNumericComparison before_min.
             * @member {Long} before_min
             * @memberof minimir.v1.SkillNumericComparison
             * @instance
             */
            SkillNumericComparison.prototype.before_min = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * SkillNumericComparison before_max.
             * @member {Long} before_max
             * @memberof minimir.v1.SkillNumericComparison
             * @instance
             */
            SkillNumericComparison.prototype.before_max = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * SkillNumericComparison after_min.
             * @member {Long} after_min
             * @memberof minimir.v1.SkillNumericComparison
             * @instance
             */
            SkillNumericComparison.prototype.after_min = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * SkillNumericComparison after_max.
             * @member {Long} after_max
             * @memberof minimir.v1.SkillNumericComparison
             * @instance
             */
            SkillNumericComparison.prototype.after_max = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * Creates a new SkillNumericComparison instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {minimir.v1.SkillNumericComparison.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SkillNumericComparison} SkillNumericComparison instance
             * @type {{
             *   (properties: minimir.v1.SkillNumericComparison.$Shape): minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape;
             *   (properties?: minimir.v1.SkillNumericComparison.$Properties): minimir.v1.SkillNumericComparison;
             * }}
             */
            SkillNumericComparison.create = function(properties) {
                return new SkillNumericComparison(properties);
            };

            /**
             * Encodes the specified SkillNumericComparison message. Does not implicitly {@link minimir.v1.SkillNumericComparison.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {minimir.v1.SkillNumericComparison.$Properties} message SkillNumericComparison message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillNumericComparison.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.key != null && $Object.hasOwnProperty.call(message, "key") && message.key !== "")
                    writer.uint32(/* id 1, wireType 2 =*/10).string(message.key);
                if (message.before_min != null && $Object.hasOwnProperty.call(message, "before_min") && (typeof message.before_min === "object" ? message.before_min.low || message.before_min.high : message.before_min !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).int64(message.before_min);
                if (message.before_max != null && $Object.hasOwnProperty.call(message, "before_max") && (typeof message.before_max === "object" ? message.before_max.low || message.before_max.high : message.before_max !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).int64(message.before_max);
                if (message.after_min != null && $Object.hasOwnProperty.call(message, "after_min") && (typeof message.after_min === "object" ? message.after_min.low || message.after_min.high : message.after_min !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).int64(message.after_min);
                if (message.after_max != null && $Object.hasOwnProperty.call(message, "after_max") && (typeof message.after_max === "object" ? message.after_max.low || message.after_max.high : message.after_max !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).int64(message.after_max);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SkillNumericComparison message, length delimited. Does not implicitly {@link minimir.v1.SkillNumericComparison.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {minimir.v1.SkillNumericComparison.$Properties} message SkillNumericComparison message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillNumericComparison.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SkillNumericComparison message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape} SkillNumericComparison
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillNumericComparison.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SkillNumericComparison();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.key = value;
                            else
                                delete message.key;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.before_min = value;
                            else
                                delete message.before_min;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.before_max = value;
                            else
                                delete message.before_max;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.after_min = value;
                            else
                                delete message.after_min;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.after_max = value;
                            else
                                delete message.after_max;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SkillNumericComparison message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillNumericComparison & minimir.v1.SkillNumericComparison.$Shape} SkillNumericComparison
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillNumericComparison.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SkillNumericComparison message.
             * @function verify
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SkillNumericComparison.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.key != null && $Object.hasOwnProperty.call(message, "key"))
                    if (!$util.isString(message.key))
                        return "key: string expected";
                if (message.before_min != null && $Object.hasOwnProperty.call(message, "before_min"))
                    if (!$util.isInteger(message.before_min) && !(message.before_min && $util.isInteger(message.before_min.low) && $util.isInteger(message.before_min.high)))
                        return "before_min: integer|Long expected";
                if (message.before_max != null && $Object.hasOwnProperty.call(message, "before_max"))
                    if (!$util.isInteger(message.before_max) && !(message.before_max && $util.isInteger(message.before_max.low) && $util.isInteger(message.before_max.high)))
                        return "before_max: integer|Long expected";
                if (message.after_min != null && $Object.hasOwnProperty.call(message, "after_min"))
                    if (!$util.isInteger(message.after_min) && !(message.after_min && $util.isInteger(message.after_min.low) && $util.isInteger(message.after_min.high)))
                        return "after_min: integer|Long expected";
                if (message.after_max != null && $Object.hasOwnProperty.call(message, "after_max"))
                    if (!$util.isInteger(message.after_max) && !(message.after_max && $util.isInteger(message.after_max.low) && $util.isInteger(message.after_max.high)))
                        return "after_max: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for SkillNumericComparison
             * @function getTypeUrl
             * @memberof minimir.v1.SkillNumericComparison
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SkillNumericComparison.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SkillNumericComparison";
            };

            return SkillNumericComparison;
        })();

        v1.SkillAssetPreview = (function() {

            /**
             * Properties of a SkillAssetPreview.
             * @typedef {Object} minimir.v1.SkillAssetPreview.$Properties
             * @property {Uint8Array|null} [preview_id] SkillAssetPreview preview_id
             * @property {number|null} [kind] SkillAssetPreview kind
             * @property {Long|null} [rule_version] SkillAssetPreview rule_version
             * @property {string|null} [config_hash] SkillAssetPreview config_hash
             * @property {Long|null} [role_revision] SkillAssetPreview role_revision
             * @property {Long|null} [expires_at_unix_ms] SkillAssetPreview expires_at_unix_ms
             * @property {number|null} [skill_id] SkillAssetPreview skill_id
             * @property {number|null} [from_level] SkillAssetPreview from_level
             * @property {number|null} [to_level] SkillAssetPreview to_level
             * @property {minimir.v1.WalletState.$Properties|null} [spend] SkillAssetPreview spend
             * @property {Array.<minimir.v1.SkillMaterialView.$Properties>|null} [materials] SkillAssetPreview materials
             * @property {Array.<minimir.v1.ItemView.$Properties>|null} [outputs] SkillAssetPreview outputs
             * @property {Array.<minimir.v1.SkillNumericComparison.$Properties>|null} [stats] SkillAssetPreview stats
             * @property {number|null} [offer_id] SkillAssetPreview offer_id
             * @property {Long|null} [quantity] SkillAssetPreview quantity
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SkillAssetPreview.
             * @memberof minimir.v1
             * @interface ISkillAssetPreview
             * @augments minimir.v1.SkillAssetPreview.$Properties
             * @deprecated Use minimir.v1.SkillAssetPreview.$Properties instead.
             */

            /**
             * Shape of a SkillAssetPreview.
             * @typedef {minimir.v1.SkillAssetPreview.$Properties} minimir.v1.SkillAssetPreview.$Shape
             */

            /**
             * Constructs a new SkillAssetPreview.
             * @memberof minimir.v1
             * @classdesc Represents a SkillAssetPreview.
             * @constructor
             * @param {minimir.v1.SkillAssetPreview.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SkillAssetPreview = function (properties) {
                this.materials = [];
                this.outputs = [];
                this.stats = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SkillAssetPreview preview_id.
             * @member {Uint8Array} preview_id
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.preview_id = $util.newBuffer([]);

            /**
             * SkillAssetPreview kind.
             * @member {number} kind
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.kind = 0;

            /**
             * SkillAssetPreview rule_version.
             * @member {Long} rule_version
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.rule_version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SkillAssetPreview config_hash.
             * @member {string} config_hash
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.config_hash = "";

            /**
             * SkillAssetPreview role_revision.
             * @member {Long} role_revision
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.role_revision = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SkillAssetPreview expires_at_unix_ms.
             * @member {Long} expires_at_unix_ms
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.expires_at_unix_ms = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * SkillAssetPreview skill_id.
             * @member {number} skill_id
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.skill_id = 0;

            /**
             * SkillAssetPreview from_level.
             * @member {number} from_level
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.from_level = 0;

            /**
             * SkillAssetPreview to_level.
             * @member {number} to_level
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.to_level = 0;

            /**
             * SkillAssetPreview spend.
             * @member {minimir.v1.WalletState.$Properties|null|undefined} spend
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.spend = null;

            /**
             * SkillAssetPreview materials.
             * @member {Array.<minimir.v1.SkillMaterialView.$Properties>} materials
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.materials = $util.emptyArray;

            /**
             * SkillAssetPreview outputs.
             * @member {Array.<minimir.v1.ItemView.$Properties>} outputs
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.outputs = $util.emptyArray;

            /**
             * SkillAssetPreview stats.
             * @member {Array.<minimir.v1.SkillNumericComparison.$Properties>} stats
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.stats = $util.emptyArray;

            /**
             * SkillAssetPreview offer_id.
             * @member {number} offer_id
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.offer_id = 0;

            /**
             * SkillAssetPreview quantity.
             * @member {Long} quantity
             * @memberof minimir.v1.SkillAssetPreview
             * @instance
             */
            SkillAssetPreview.prototype.quantity = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * Creates a new SkillAssetPreview instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {minimir.v1.SkillAssetPreview.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SkillAssetPreview} SkillAssetPreview instance
             * @type {{
             *   (properties: minimir.v1.SkillAssetPreview.$Shape): minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape;
             *   (properties?: minimir.v1.SkillAssetPreview.$Properties): minimir.v1.SkillAssetPreview;
             * }}
             */
            SkillAssetPreview.create = function(properties) {
                return new SkillAssetPreview(properties);
            };

            /**
             * Encodes the specified SkillAssetPreview message. Does not implicitly {@link minimir.v1.SkillAssetPreview.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {minimir.v1.SkillAssetPreview.$Properties} message SkillAssetPreview message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetPreview.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.preview_id != null && $Object.hasOwnProperty.call(message, "preview_id") && message.preview_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.preview_id);
                if (message.kind != null && $Object.hasOwnProperty.call(message, "kind") && message.kind !== 0)
                    writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.kind);
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version") && (typeof message.rule_version === "object" ? message.rule_version.low || message.rule_version.high : message.rule_version !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.rule_version);
                if (message.config_hash != null && $Object.hasOwnProperty.call(message, "config_hash") && message.config_hash !== "")
                    writer.uint32(/* id 4, wireType 2 =*/34).string(message.config_hash);
                if (message.role_revision != null && $Object.hasOwnProperty.call(message, "role_revision") && (typeof message.role_revision === "object" ? message.role_revision.low || message.role_revision.high : message.role_revision !== 0))
                    writer.uint32(/* id 5, wireType 0 =*/40).uint64(message.role_revision);
                if (message.expires_at_unix_ms != null && $Object.hasOwnProperty.call(message, "expires_at_unix_ms") && (typeof message.expires_at_unix_ms === "object" ? message.expires_at_unix_ms.low || message.expires_at_unix_ms.high : message.expires_at_unix_ms !== 0))
                    writer.uint32(/* id 6, wireType 0 =*/48).uint64(message.expires_at_unix_ms);
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id") && message.skill_id !== 0)
                    writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.skill_id);
                if (message.from_level != null && $Object.hasOwnProperty.call(message, "from_level") && message.from_level !== 0)
                    writer.uint32(/* id 8, wireType 0 =*/64).uint32(message.from_level);
                if (message.to_level != null && $Object.hasOwnProperty.call(message, "to_level") && message.to_level !== 0)
                    writer.uint32(/* id 9, wireType 0 =*/72).uint32(message.to_level);
                if (message.spend != null && $Object.hasOwnProperty.call(message, "spend"))
                    $root.minimir.v1.WalletState.encode(message.spend, writer.uint32(/* id 10, wireType 2 =*/82).fork(), _depth + 1).ldelim();
                if (message.materials != null && message.materials.length)
                    for (var i = 0; i < message.materials.length; ++i)
                        $root.minimir.v1.SkillMaterialView.encode(message.materials[i], writer.uint32(/* id 11, wireType 2 =*/90).fork(), _depth + 1).ldelim();
                if (message.outputs != null && message.outputs.length)
                    for (var i = 0; i < message.outputs.length; ++i)
                        $root.minimir.v1.ItemView.encode(message.outputs[i], writer.uint32(/* id 12, wireType 2 =*/98).fork(), _depth + 1).ldelim();
                if (message.stats != null && message.stats.length)
                    for (var i = 0; i < message.stats.length; ++i)
                        $root.minimir.v1.SkillNumericComparison.encode(message.stats[i], writer.uint32(/* id 13, wireType 2 =*/106).fork(), _depth + 1).ldelim();
                if (message.offer_id != null && $Object.hasOwnProperty.call(message, "offer_id") && message.offer_id !== 0)
                    writer.uint32(/* id 14, wireType 0 =*/112).uint32(message.offer_id);
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity") && (typeof message.quantity === "object" ? message.quantity.low || message.quantity.high : message.quantity !== 0))
                    writer.uint32(/* id 15, wireType 0 =*/120).int64(message.quantity);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SkillAssetPreview message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetPreview.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {minimir.v1.SkillAssetPreview.$Properties} message SkillAssetPreview message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetPreview.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SkillAssetPreview message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape} SkillAssetPreview
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetPreview.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SkillAssetPreview();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.preview_id = value;
                            else
                                delete message.preview_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.kind = value;
                            else
                                delete message.kind;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.rule_version = value;
                            else
                                delete message.rule_version;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.stringVerify()).length)
                                message.config_hash = value;
                            else
                                delete message.config_hash;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.role_revision = value;
                            else
                                delete message.role_revision;
                            continue;
                        }
                    case 6: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.expires_at_unix_ms = value;
                            else
                                delete message.expires_at_unix_ms;
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.skill_id = value;
                            else
                                delete message.skill_id;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.from_level = value;
                            else
                                delete message.from_level;
                            continue;
                        }
                    case 9: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.to_level = value;
                            else
                                delete message.to_level;
                            continue;
                        }
                    case 10: {
                            if (wireType !== 2)
                                break;
                            message.spend = $root.minimir.v1.WalletState.decode(reader, reader.size(), $undefined, _depth + 1, message.spend);
                            continue;
                        }
                    case 11: {
                            if (wireType !== 2)
                                break;
                            if (!(message.materials && message.materials.length))
                                message.materials = [];
                            message.materials.push($root.minimir.v1.SkillMaterialView.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 12: {
                            if (wireType !== 2)
                                break;
                            if (!(message.outputs && message.outputs.length))
                                message.outputs = [];
                            message.outputs.push($root.minimir.v1.ItemView.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 13: {
                            if (wireType !== 2)
                                break;
                            if (!(message.stats && message.stats.length))
                                message.stats = [];
                            message.stats.push($root.minimir.v1.SkillNumericComparison.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 14: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.offer_id = value;
                            else
                                delete message.offer_id;
                            continue;
                        }
                    case 15: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.quantity = value;
                            else
                                delete message.quantity;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SkillAssetPreview message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetPreview & minimir.v1.SkillAssetPreview.$Shape} SkillAssetPreview
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetPreview.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SkillAssetPreview message.
             * @function verify
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SkillAssetPreview.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.preview_id != null && $Object.hasOwnProperty.call(message, "preview_id"))
                    if (!(message.preview_id && typeof message.preview_id.length === "number" || $util.isString(message.preview_id)))
                        return "preview_id: buffer expected";
                if (message.kind != null && $Object.hasOwnProperty.call(message, "kind"))
                    if (!$util.isInteger(message.kind))
                        return "kind: integer expected";
                if (message.rule_version != null && $Object.hasOwnProperty.call(message, "rule_version"))
                    if (!$util.isInteger(message.rule_version) && !(message.rule_version && $util.isInteger(message.rule_version.low) && $util.isInteger(message.rule_version.high)))
                        return "rule_version: integer|Long expected";
                if (message.config_hash != null && $Object.hasOwnProperty.call(message, "config_hash"))
                    if (!$util.isString(message.config_hash))
                        return "config_hash: string expected";
                if (message.role_revision != null && $Object.hasOwnProperty.call(message, "role_revision"))
                    if (!$util.isInteger(message.role_revision) && !(message.role_revision && $util.isInteger(message.role_revision.low) && $util.isInteger(message.role_revision.high)))
                        return "role_revision: integer|Long expected";
                if (message.expires_at_unix_ms != null && $Object.hasOwnProperty.call(message, "expires_at_unix_ms"))
                    if (!$util.isInteger(message.expires_at_unix_ms) && !(message.expires_at_unix_ms && $util.isInteger(message.expires_at_unix_ms.low) && $util.isInteger(message.expires_at_unix_ms.high)))
                        return "expires_at_unix_ms: integer|Long expected";
                if (message.skill_id != null && $Object.hasOwnProperty.call(message, "skill_id"))
                    if (!$util.isInteger(message.skill_id))
                        return "skill_id: integer expected";
                if (message.from_level != null && $Object.hasOwnProperty.call(message, "from_level"))
                    if (!$util.isInteger(message.from_level))
                        return "from_level: integer expected";
                if (message.to_level != null && $Object.hasOwnProperty.call(message, "to_level"))
                    if (!$util.isInteger(message.to_level))
                        return "to_level: integer expected";
                if (message.spend != null && $Object.hasOwnProperty.call(message, "spend")) {
                    var error = $root.minimir.v1.WalletState.verify(message.spend, _depth + 1);
                    if (error)
                        return "spend." + error;
                }
                if (message.materials != null && $Object.hasOwnProperty.call(message, "materials")) {
                    if (!$Array.isArray(message.materials))
                        return "materials: array expected";
                    for (var i = 0; i < message.materials.length; ++i) {
                        var error = $root.minimir.v1.SkillMaterialView.verify(message.materials[i], _depth + 1);
                        if (error)
                            return "materials." + error;
                    }
                }
                if (message.outputs != null && $Object.hasOwnProperty.call(message, "outputs")) {
                    if (!$Array.isArray(message.outputs))
                        return "outputs: array expected";
                    for (var i = 0; i < message.outputs.length; ++i) {
                        var error = $root.minimir.v1.ItemView.verify(message.outputs[i], _depth + 1);
                        if (error)
                            return "outputs." + error;
                    }
                }
                if (message.stats != null && $Object.hasOwnProperty.call(message, "stats")) {
                    if (!$Array.isArray(message.stats))
                        return "stats: array expected";
                    for (var i = 0; i < message.stats.length; ++i) {
                        var error = $root.minimir.v1.SkillNumericComparison.verify(message.stats[i], _depth + 1);
                        if (error)
                            return "stats." + error;
                    }
                }
                if (message.offer_id != null && $Object.hasOwnProperty.call(message, "offer_id"))
                    if (!$util.isInteger(message.offer_id))
                        return "offer_id: integer expected";
                if (message.quantity != null && $Object.hasOwnProperty.call(message, "quantity"))
                    if (!$util.isInteger(message.quantity) && !(message.quantity && $util.isInteger(message.quantity.low) && $util.isInteger(message.quantity.high)))
                        return "quantity: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for SkillAssetPreview
             * @function getTypeUrl
             * @memberof minimir.v1.SkillAssetPreview
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SkillAssetPreview.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SkillAssetPreview";
            };

            return SkillAssetPreview;
        })();

        v1.SkillAssetCommitRequest = (function() {

            /**
             * Properties of a SkillAssetCommitRequest.
             * @typedef {Object} minimir.v1.SkillAssetCommitRequest.$Properties
             * @property {Uint8Array|null} [preview_id] SkillAssetCommitRequest preview_id
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] SkillAssetCommitRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a SkillAssetCommitRequest.
             * @memberof minimir.v1
             * @interface ISkillAssetCommitRequest
             * @augments minimir.v1.SkillAssetCommitRequest.$Properties
             * @deprecated Use minimir.v1.SkillAssetCommitRequest.$Properties instead.
             */

            /**
             * Shape of a SkillAssetCommitRequest.
             * @typedef {minimir.v1.SkillAssetCommitRequest.$Properties} minimir.v1.SkillAssetCommitRequest.$Shape
             */

            /**
             * Constructs a new SkillAssetCommitRequest.
             * @memberof minimir.v1
             * @classdesc Represents a SkillAssetCommitRequest.
             * @constructor
             * @param {minimir.v1.SkillAssetCommitRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var SkillAssetCommitRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * SkillAssetCommitRequest preview_id.
             * @member {Uint8Array} preview_id
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @instance
             */
            SkillAssetCommitRequest.prototype.preview_id = $util.newBuffer([]);

            /**
             * SkillAssetCommitRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @instance
             */
            SkillAssetCommitRequest.prototype.operation = null;

            /**
             * Creates a new SkillAssetCommitRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {minimir.v1.SkillAssetCommitRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.SkillAssetCommitRequest} SkillAssetCommitRequest instance
             * @type {{
             *   (properties: minimir.v1.SkillAssetCommitRequest.$Shape): minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape;
             *   (properties?: minimir.v1.SkillAssetCommitRequest.$Properties): minimir.v1.SkillAssetCommitRequest;
             * }}
             */
            SkillAssetCommitRequest.create = function(properties) {
                return new SkillAssetCommitRequest(properties);
            };

            /**
             * Encodes the specified SkillAssetCommitRequest message. Does not implicitly {@link minimir.v1.SkillAssetCommitRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {minimir.v1.SkillAssetCommitRequest.$Properties} message SkillAssetCommitRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetCommitRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.preview_id != null && $Object.hasOwnProperty.call(message, "preview_id") && message.preview_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.preview_id);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 2, wireType 2 =*/18).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified SkillAssetCommitRequest message, length delimited. Does not implicitly {@link minimir.v1.SkillAssetCommitRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {minimir.v1.SkillAssetCommitRequest.$Properties} message SkillAssetCommitRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            SkillAssetCommitRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a SkillAssetCommitRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape} SkillAssetCommitRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetCommitRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.SkillAssetCommitRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.preview_id = value;
                            else
                                delete message.preview_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a SkillAssetCommitRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.SkillAssetCommitRequest & minimir.v1.SkillAssetCommitRequest.$Shape} SkillAssetCommitRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            SkillAssetCommitRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a SkillAssetCommitRequest message.
             * @function verify
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            SkillAssetCommitRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.preview_id != null && $Object.hasOwnProperty.call(message, "preview_id"))
                    if (!(message.preview_id && typeof message.preview_id.length === "number" || $util.isString(message.preview_id)))
                        return "preview_id: buffer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for SkillAssetCommitRequest
             * @function getTypeUrl
             * @memberof minimir.v1.SkillAssetCommitRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            SkillAssetCommitRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.SkillAssetCommitRequest";
            };

            return SkillAssetCommitRequest;
        })();

        v1.ClaimHostingTimeRequest = (function() {

            /**
             * Properties of a ClaimHostingTimeRequest.
             * @typedef {Object} minimir.v1.ClaimHostingTimeRequest.$Properties
             * @property {number|null} [benefit_kind] ClaimHostingTimeRequest benefit_kind
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] ClaimHostingTimeRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a ClaimHostingTimeRequest.
             * @memberof minimir.v1
             * @interface IClaimHostingTimeRequest
             * @augments minimir.v1.ClaimHostingTimeRequest.$Properties
             * @deprecated Use minimir.v1.ClaimHostingTimeRequest.$Properties instead.
             */

            /**
             * Shape of a ClaimHostingTimeRequest.
             * @typedef {minimir.v1.ClaimHostingTimeRequest.$Properties} minimir.v1.ClaimHostingTimeRequest.$Shape
             */

            /**
             * Constructs a new ClaimHostingTimeRequest.
             * @memberof minimir.v1
             * @classdesc Represents a ClaimHostingTimeRequest.
             * @constructor
             * @param {minimir.v1.ClaimHostingTimeRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var ClaimHostingTimeRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * ClaimHostingTimeRequest benefit_kind.
             * @member {number} benefit_kind
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @instance
             */
            ClaimHostingTimeRequest.prototype.benefit_kind = 0;

            /**
             * ClaimHostingTimeRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @instance
             */
            ClaimHostingTimeRequest.prototype.operation = null;

            /**
             * Creates a new ClaimHostingTimeRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {minimir.v1.ClaimHostingTimeRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.ClaimHostingTimeRequest} ClaimHostingTimeRequest instance
             * @type {{
             *   (properties: minimir.v1.ClaimHostingTimeRequest.$Shape): minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape;
             *   (properties?: minimir.v1.ClaimHostingTimeRequest.$Properties): minimir.v1.ClaimHostingTimeRequest;
             * }}
             */
            ClaimHostingTimeRequest.create = function(properties) {
                return new ClaimHostingTimeRequest(properties);
            };

            /**
             * Encodes the specified ClaimHostingTimeRequest message. Does not implicitly {@link minimir.v1.ClaimHostingTimeRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {minimir.v1.ClaimHostingTimeRequest.$Properties} message ClaimHostingTimeRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ClaimHostingTimeRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.benefit_kind != null && $Object.hasOwnProperty.call(message, "benefit_kind") && message.benefit_kind !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.benefit_kind);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 2, wireType 2 =*/18).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified ClaimHostingTimeRequest message, length delimited. Does not implicitly {@link minimir.v1.ClaimHostingTimeRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {minimir.v1.ClaimHostingTimeRequest.$Properties} message ClaimHostingTimeRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            ClaimHostingTimeRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a ClaimHostingTimeRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape} ClaimHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ClaimHostingTimeRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.ClaimHostingTimeRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.benefit_kind = value;
                            else
                                delete message.benefit_kind;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a ClaimHostingTimeRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.ClaimHostingTimeRequest & minimir.v1.ClaimHostingTimeRequest.$Shape} ClaimHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            ClaimHostingTimeRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a ClaimHostingTimeRequest message.
             * @function verify
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            ClaimHostingTimeRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.benefit_kind != null && $Object.hasOwnProperty.call(message, "benefit_kind"))
                    if (!$util.isInteger(message.benefit_kind))
                        return "benefit_kind: integer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for ClaimHostingTimeRequest
             * @function getTypeUrl
             * @memberof minimir.v1.ClaimHostingTimeRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            ClaimHostingTimeRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.ClaimHostingTimeRequest";
            };

            return ClaimHostingTimeRequest;
        })();

        v1.BuyHostingTimeRequest = (function() {

            /**
             * Properties of a BuyHostingTimeRequest.
             * @typedef {Object} minimir.v1.BuyHostingTimeRequest.$Properties
             * @property {number|null} [requested_seconds] BuyHostingTimeRequest requested_seconds
             * @property {minimir.v1.OperationContext.$Properties|null} [operation] BuyHostingTimeRequest operation
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a BuyHostingTimeRequest.
             * @memberof minimir.v1
             * @interface IBuyHostingTimeRequest
             * @augments minimir.v1.BuyHostingTimeRequest.$Properties
             * @deprecated Use minimir.v1.BuyHostingTimeRequest.$Properties instead.
             */

            /**
             * Shape of a BuyHostingTimeRequest.
             * @typedef {minimir.v1.BuyHostingTimeRequest.$Properties} minimir.v1.BuyHostingTimeRequest.$Shape
             */

            /**
             * Constructs a new BuyHostingTimeRequest.
             * @memberof minimir.v1
             * @classdesc Represents a BuyHostingTimeRequest.
             * @constructor
             * @param {minimir.v1.BuyHostingTimeRequest.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var BuyHostingTimeRequest = function (properties) {
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * BuyHostingTimeRequest requested_seconds.
             * @member {number} requested_seconds
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @instance
             */
            BuyHostingTimeRequest.prototype.requested_seconds = 0;

            /**
             * BuyHostingTimeRequest operation.
             * @member {minimir.v1.OperationContext.$Properties|null|undefined} operation
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @instance
             */
            BuyHostingTimeRequest.prototype.operation = null;

            /**
             * Creates a new BuyHostingTimeRequest instance using the specified properties.
             * @function create
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {minimir.v1.BuyHostingTimeRequest.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.BuyHostingTimeRequest} BuyHostingTimeRequest instance
             * @type {{
             *   (properties: minimir.v1.BuyHostingTimeRequest.$Shape): minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape;
             *   (properties?: minimir.v1.BuyHostingTimeRequest.$Properties): minimir.v1.BuyHostingTimeRequest;
             * }}
             */
            BuyHostingTimeRequest.create = function(properties) {
                return new BuyHostingTimeRequest(properties);
            };

            /**
             * Encodes the specified BuyHostingTimeRequest message. Does not implicitly {@link minimir.v1.BuyHostingTimeRequest.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {minimir.v1.BuyHostingTimeRequest.$Properties} message BuyHostingTimeRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            BuyHostingTimeRequest.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.requested_seconds != null && $Object.hasOwnProperty.call(message, "requested_seconds") && message.requested_seconds !== 0)
                    writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.requested_seconds);
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation"))
                    $root.minimir.v1.OperationContext.encode(message.operation, writer.uint32(/* id 2, wireType 2 =*/18).fork(), _depth + 1).ldelim();
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified BuyHostingTimeRequest message, length delimited. Does not implicitly {@link minimir.v1.BuyHostingTimeRequest.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {minimir.v1.BuyHostingTimeRequest.$Properties} message BuyHostingTimeRequest message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            BuyHostingTimeRequest.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a BuyHostingTimeRequest message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape} BuyHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            BuyHostingTimeRequest.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.BuyHostingTimeRequest();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.requested_seconds = value;
                            else
                                delete message.requested_seconds;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 2)
                                break;
                            message.operation = $root.minimir.v1.OperationContext.decode(reader, reader.size(), $undefined, _depth + 1, message.operation);
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a BuyHostingTimeRequest message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.BuyHostingTimeRequest & minimir.v1.BuyHostingTimeRequest.$Shape} BuyHostingTimeRequest
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            BuyHostingTimeRequest.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a BuyHostingTimeRequest message.
             * @function verify
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            BuyHostingTimeRequest.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.requested_seconds != null && $Object.hasOwnProperty.call(message, "requested_seconds"))
                    if (!$util.isInteger(message.requested_seconds))
                        return "requested_seconds: integer expected";
                if (message.operation != null && $Object.hasOwnProperty.call(message, "operation")) {
                    var error = $root.minimir.v1.OperationContext.verify(message.operation, _depth + 1);
                    if (error)
                        return "operation." + error;
                }
                return null;
            };

            /**
             * Gets the type url for BuyHostingTimeRequest
             * @function getTypeUrl
             * @memberof minimir.v1.BuyHostingTimeRequest
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            BuyHostingTimeRequest.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.BuyHostingTimeRequest";
            };

            return BuyHostingTimeRequest;
        })();

        v1.HostingReport = (function() {

            /**
             * Properties of a HostingReport.
             * @typedef {Object} minimir.v1.HostingReport.$Properties
             * @property {Uint8Array|null} [hosting_session_id] HostingReport hosting_session_id
             * @property {Long|null} [used_seconds] HostingReport used_seconds
             * @property {Long|null} [kills] HostingReport kills
             * @property {Long|null} [experience] HostingReport experience
             * @property {minimir.v1.WalletState.$Properties|null} [earned] HostingReport earned
             * @property {Array.<minimir.v1.ItemView.$Properties>|null} [drops] HostingReport drops
             * @property {number|null} [stop_reason] HostingReport stop_reason
             * @property {Long|null} [settlement_sequence] HostingReport settlement_sequence
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */

            /**
             * Properties of a HostingReport.
             * @memberof minimir.v1
             * @interface IHostingReport
             * @augments minimir.v1.HostingReport.$Properties
             * @deprecated Use minimir.v1.HostingReport.$Properties instead.
             */

            /**
             * Shape of a HostingReport.
             * @typedef {minimir.v1.HostingReport.$Properties} minimir.v1.HostingReport.$Shape
             */

            /**
             * Constructs a new HostingReport.
             * @memberof minimir.v1
             * @classdesc Represents a HostingReport.
             * @constructor
             * @param {minimir.v1.HostingReport.$Properties=} [properties] Properties to set
             * @property {Array.<Uint8Array>} [$unknowns] Unknown fields preserved while decoding when enabled
             */
            var HostingReport = function (properties) {
                this.drops = [];
                if (properties)
                    for (var keys = $Object.keys(properties), i = 0; i < keys.length; ++i)
                        if (properties[keys[i]] != null && keys[i] !== "__proto__")
                            this[keys[i]] = properties[keys[i]];
            };

            /**
             * HostingReport hosting_session_id.
             * @member {Uint8Array} hosting_session_id
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.hosting_session_id = $util.newBuffer([]);

            /**
             * HostingReport used_seconds.
             * @member {Long} used_seconds
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.used_seconds = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * HostingReport kills.
             * @member {Long} kills
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.kills = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * HostingReport experience.
             * @member {Long} experience
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.experience = $util.Long ? $util.Long.fromBits(0,0,false) : 0;

            /**
             * HostingReport earned.
             * @member {minimir.v1.WalletState.$Properties|null|undefined} earned
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.earned = null;

            /**
             * HostingReport drops.
             * @member {Array.<minimir.v1.ItemView.$Properties>} drops
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.drops = $util.emptyArray;

            /**
             * HostingReport stop_reason.
             * @member {number} stop_reason
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.stop_reason = 0;

            /**
             * HostingReport settlement_sequence.
             * @member {Long} settlement_sequence
             * @memberof minimir.v1.HostingReport
             * @instance
             */
            HostingReport.prototype.settlement_sequence = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

            /**
             * Creates a new HostingReport instance using the specified properties.
             * @function create
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {minimir.v1.HostingReport.$Properties=} [properties] Properties to set
             * @returns {minimir.v1.HostingReport} HostingReport instance
             * @type {{
             *   (properties: minimir.v1.HostingReport.$Shape): minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape;
             *   (properties?: minimir.v1.HostingReport.$Properties): minimir.v1.HostingReport;
             * }}
             */
            HostingReport.create = function(properties) {
                return new HostingReport(properties);
            };

            /**
             * Encodes the specified HostingReport message. Does not implicitly {@link minimir.v1.HostingReport.verify|verify} messages.
             * @function encode
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {minimir.v1.HostingReport.$Properties} message HostingReport message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HostingReport.encode = function (message, writer, _depth) {
                if (!writer)
                    writer = $Writer.create();
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    throw $Error("max depth exceeded");
                if (message.hosting_session_id != null && $Object.hasOwnProperty.call(message, "hosting_session_id") && message.hosting_session_id.length)
                    writer.uint32(/* id 1, wireType 2 =*/10).bytes(message.hosting_session_id);
                if (message.used_seconds != null && $Object.hasOwnProperty.call(message, "used_seconds") && (typeof message.used_seconds === "object" ? message.used_seconds.low || message.used_seconds.high : message.used_seconds !== 0))
                    writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.used_seconds);
                if (message.kills != null && $Object.hasOwnProperty.call(message, "kills") && (typeof message.kills === "object" ? message.kills.low || message.kills.high : message.kills !== 0))
                    writer.uint32(/* id 3, wireType 0 =*/24).uint64(message.kills);
                if (message.experience != null && $Object.hasOwnProperty.call(message, "experience") && (typeof message.experience === "object" ? message.experience.low || message.experience.high : message.experience !== 0))
                    writer.uint32(/* id 4, wireType 0 =*/32).int64(message.experience);
                if (message.earned != null && $Object.hasOwnProperty.call(message, "earned"))
                    $root.minimir.v1.WalletState.encode(message.earned, writer.uint32(/* id 5, wireType 2 =*/42).fork(), _depth + 1).ldelim();
                if (message.drops != null && message.drops.length)
                    for (var i = 0; i < message.drops.length; ++i)
                        $root.minimir.v1.ItemView.encode(message.drops[i], writer.uint32(/* id 6, wireType 2 =*/50).fork(), _depth + 1).ldelim();
                if (message.stop_reason != null && $Object.hasOwnProperty.call(message, "stop_reason") && message.stop_reason !== 0)
                    writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.stop_reason);
                if (message.settlement_sequence != null && $Object.hasOwnProperty.call(message, "settlement_sequence") && (typeof message.settlement_sequence === "object" ? message.settlement_sequence.low || message.settlement_sequence.high : message.settlement_sequence !== 0))
                    writer.uint32(/* id 8, wireType 0 =*/64).uint64(message.settlement_sequence);
                if (message.$unknowns != null && $Object.hasOwnProperty.call(message, "$unknowns"))
                    for (var i = 0; i < message.$unknowns.length; ++i)
                        writer.raw(message.$unknowns[i]);
                return writer;
            };

            /**
             * Encodes the specified HostingReport message, length delimited. Does not implicitly {@link minimir.v1.HostingReport.verify|verify} messages.
             * @function encodeDelimited
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {minimir.v1.HostingReport.$Properties} message HostingReport message or plain object to encode
             * @param {$protobuf.Writer} [writer] Writer to encode to
             * @returns {$protobuf.Writer} Writer
             */
            HostingReport.encodeDelimited = function(message, writer) {
                return this.encode(message, (writer || $Writer.create()).fork()).ldelim();
            };

            /**
             * Decodes a HostingReport message from the specified reader or buffer.
             * @function decode
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @param {number} [length] Message length if known beforehand
             * @returns {minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape} HostingReport
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HostingReport.decode = function (reader, length, _end, _depth, _target) {
                if (!(reader instanceof $Reader))
                    reader = $Reader.create(reader);
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $Reader.recursionLimit)
                    throw $Error("max depth exceeded");
                var end, message, value;
                if (length === $undefined)
                    end = reader.len;
                else {
                    end = reader.pos + length;
                    if (end > reader.len)
                        throw $RangeError("index out of range");
                    length = reader.len;
                    reader.len = end;
                }
                message = _target || new $root.minimir.v1.HostingReport();
                while (reader.pos < end) {
                    var start = reader.pos;
                    var tag = reader.tag();
                    if (tag === _end) {
                        _end = $undefined;
                        break;
                    }
                    var wireType = tag & 7;
                    switch (tag >>>= 3) {
                    case 1: {
                            if (wireType !== 2)
                                break;
                            if ((value = reader.bytes()).length)
                                message.hosting_session_id = value;
                            else
                                delete message.hosting_session_id;
                            continue;
                        }
                    case 2: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.used_seconds = value;
                            else
                                delete message.used_seconds;
                            continue;
                        }
                    case 3: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.kills = value;
                            else
                                delete message.kills;
                            continue;
                        }
                    case 4: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.int64()) === "object" ? value.low || value.high : value !== 0)
                                message.experience = value;
                            else
                                delete message.experience;
                            continue;
                        }
                    case 5: {
                            if (wireType !== 2)
                                break;
                            message.earned = $root.minimir.v1.WalletState.decode(reader, reader.size(), $undefined, _depth + 1, message.earned);
                            continue;
                        }
                    case 6: {
                            if (wireType !== 2)
                                break;
                            if (!(message.drops && message.drops.length))
                                message.drops = [];
                            message.drops.push($root.minimir.v1.ItemView.decode(reader, reader.size(), $undefined, _depth + 1));
                            continue;
                        }
                    case 7: {
                            if (wireType !== 0)
                                break;
                            if (value = reader.uint32())
                                message.stop_reason = value;
                            else
                                delete message.stop_reason;
                            continue;
                        }
                    case 8: {
                            if (wireType !== 0)
                                break;
                            if (typeof (value = reader.uint64()) === "object" ? value.low || value.high : value !== 0)
                                message.settlement_sequence = value;
                            else
                                delete message.settlement_sequence;
                            continue;
                        }
                    }
                    reader.skipType(wireType, _depth, tag);
                    if (!reader.discardUnknown) {
                        $util.makeProp(message, "$unknowns", false);
                        (message.$unknowns || (message.$unknowns = [])).push(reader.raw(start, reader.pos));
                    }
                }
                if (length !== $undefined) {
                    if (reader.pos !== end)
                        throw $RangeError("index out of range");
                    reader.len = length;
                }
                if (_end !== $undefined)
                    throw $Error("missing end group");
                return message;
            };

            /**
             * Decodes a HostingReport message from the specified reader or buffer, length delimited.
             * @function decodeDelimited
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
             * @returns {minimir.v1.HostingReport & minimir.v1.HostingReport.$Shape} HostingReport
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            HostingReport.decodeDelimited = function(reader) {
                if (!(reader instanceof $Reader))
                    reader = new $Reader(reader);
                return this.decode(reader, reader.size());
            };

            /**
             * Verifies a HostingReport message.
             * @function verify
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {Object.<string,*>} message Plain object to verify
             * @returns {string|null} `null` if valid, otherwise the reason why it is not
             */
            HostingReport.verify = function (message, _depth) {
                if (typeof message !== "object" || message === null)
                    return "object expected";
                if (_depth === $undefined)
                    _depth = 0;
                if (_depth > $util.recursionLimit)
                    return "max depth exceeded";
                if (message.hosting_session_id != null && $Object.hasOwnProperty.call(message, "hosting_session_id"))
                    if (!(message.hosting_session_id && typeof message.hosting_session_id.length === "number" || $util.isString(message.hosting_session_id)))
                        return "hosting_session_id: buffer expected";
                if (message.used_seconds != null && $Object.hasOwnProperty.call(message, "used_seconds"))
                    if (!$util.isInteger(message.used_seconds) && !(message.used_seconds && $util.isInteger(message.used_seconds.low) && $util.isInteger(message.used_seconds.high)))
                        return "used_seconds: integer|Long expected";
                if (message.kills != null && $Object.hasOwnProperty.call(message, "kills"))
                    if (!$util.isInteger(message.kills) && !(message.kills && $util.isInteger(message.kills.low) && $util.isInteger(message.kills.high)))
                        return "kills: integer|Long expected";
                if (message.experience != null && $Object.hasOwnProperty.call(message, "experience"))
                    if (!$util.isInteger(message.experience) && !(message.experience && $util.isInteger(message.experience.low) && $util.isInteger(message.experience.high)))
                        return "experience: integer|Long expected";
                if (message.earned != null && $Object.hasOwnProperty.call(message, "earned")) {
                    var error = $root.minimir.v1.WalletState.verify(message.earned, _depth + 1);
                    if (error)
                        return "earned." + error;
                }
                if (message.drops != null && $Object.hasOwnProperty.call(message, "drops")) {
                    if (!$Array.isArray(message.drops))
                        return "drops: array expected";
                    for (var i = 0; i < message.drops.length; ++i) {
                        var error = $root.minimir.v1.ItemView.verify(message.drops[i], _depth + 1);
                        if (error)
                            return "drops." + error;
                    }
                }
                if (message.stop_reason != null && $Object.hasOwnProperty.call(message, "stop_reason"))
                    if (!$util.isInteger(message.stop_reason))
                        return "stop_reason: integer expected";
                if (message.settlement_sequence != null && $Object.hasOwnProperty.call(message, "settlement_sequence"))
                    if (!$util.isInteger(message.settlement_sequence) && !(message.settlement_sequence && $util.isInteger(message.settlement_sequence.low) && $util.isInteger(message.settlement_sequence.high)))
                        return "settlement_sequence: integer|Long expected";
                return null;
            };

            /**
             * Gets the type url for HostingReport
             * @function getTypeUrl
             * @memberof minimir.v1.HostingReport
             * @static
             * @param {string} [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns {string} The type url
             */
            HostingReport.getTypeUrl = function(prefix) {
                if (prefix === $undefined)
                    prefix = "type.googleapis.com";
                return prefix + "/minimir.v1.HostingReport";
            };

            return HostingReport;
        })();

        return v1;
    })();

    return minimir;
})();

module.exports = $root;
