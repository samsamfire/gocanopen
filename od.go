package canopen

import (
	"fmt"
	"log"
	"regexp"
	"strconv"

	"gopkg.in/ini.v1"
)

// def build_variable(eds, section, node_id, index, subindex=0):
//     """Creates a object dictionary entry.
//     :param eds: String stream of the eds file
//     :param section:
//     :param node_id: Node ID
//     :param index: Index of the CANOpen object
//     :param subindex: Subindex of the CANOpen object (if presente, else 0)
//     """
//     name = eds.get(section, "ParameterName")
//     var = objectdictionary.Variable(name, index, subindex)
//     var.data_type = int(eds.get(section, "DataType"), 0)
//     var.access_type = eds.get(section, "AccessType").lower()
//     if var.data_type > 0x1B:
//         # The object dictionary editor from CANFestival creates an optional object if min max values are used
//         # This optional object is then placed in the eds under the section [A0] (start point, iterates for more)
//         # The eds.get function gives us 0x00A0 now convert to String without hex representation and upper case
//         # The sub2 part is then the section where the type parameter stands
//         try:
//             var.data_type = int(eds.get("%Xsub1" % var.data_type, "DefaultValue"), 0)
//         except NoSectionError:
//             logger.warning("%s has an unknown or unsupported data type (%X)", name, var.data_type)
//             # Assume DOMAIN to force application to interpret the byte data
//             var.data_type = objectdictionary.DOMAIN

//     var.pdo_mappable = bool(int(eds.get(section, "PDOMapping", fallback="0"), 0))
//     if eds.has_option(section, "DefaultValue"):
//         try:
//             var.default_raw = eds.get(section, "DefaultValue")
//             if '$NODEID' in var.default_raw:
//                 var.relative = True
//             var.default = _convert_variable(node_id, var.data_type, eds.get(section, "DefaultValue"))
//         except ValueError:
//             pass
//     if eds.has_option(section, "ParameterValue"):
//         try:
//             var.value_raw = eds.get(section, "ParameterValue")
//             var.value = _convert_variable(node_id, var.data_type, eds.get(section, "ParameterValue"))
//         except ValueError:
//             pass
//     return var

// Create a variable from section
func createVariableFromSection(section *ini.Section, index uint16, subindex uint8) (variable *Variable) {

	variable = &Variable{}

	//These are mandatory
	variable.ParameterName = section.Key("ParameterName").Value()
	// TODO maybe add support for datatype particularities
	data_type, _ := strconv.ParseInt(section.Key("DataType").Value(), 0, 8)
	variable.DataType = DataType(data_type)

	// These are optional
	if section.HasKey("HighLimit") {
		high_limit, _ := strconv.ParseInt(section.Key("HighLimit").Value(), 0, 64)
		variable.HighLimit = int(high_limit)
	}

	if section.HasKey("LowLimit") {
		low_limit, _ := strconv.ParseInt(section.Key("HighLimit").Value(), 0, 64)
		variable.HighLimit = int(low_limit)
	}

	// TODO DefaultValue and ParameterValue should be converted using the DataType
	if section.HasKey("DefaultValue") {
		variable.DefaultValue = section.Key("DefaultValue").Value()
	}

	// if section.HasKey("HighLimit"):
	// 	variable.
	return variable
}

/*Parse EDS and return the according object dictionnary*/
func ParseEDS(file_path string) (od ObjectDictionary) {

	od = ObjectDictionary{}

	// Open the EDS file
	edsFile, err := ini.Load(file_path)
	if err != nil {
		log.Fatal(err)
	}

	// Get all the sections in the file
	sections := edsFile.Sections()

	// Get the index sections
	re := regexp.MustCompile(`^[0-9A-Fa-f]{4}$`)

	// Print out the section names
	for _, section := range sections {
		if re.MatchString(section.Name()) {
			// Add a new entry inside object dictionary
			index, err := strconv.ParseUint(section.Name(), 16, 16)
			if err != nil {
				fmt.Printf("Error trying to parse index %s", section.Name())
			}
			var object_type ObjectType
			object_type_uint64, err := strconv.ParseUint(section.Key("ObjectType").Value(), 0, 16)
			object_type = ObjectType(object_type_uint64)
			if err != nil {
				fmt.Println("Couldn't parse object type, default to VAR type")
				object_type = 7
			}

			switch object_type {
			case OBJ_VAR:
				// Build a variable type entry :
				variable := createVariableFromSection(section, uint16(index), 0)
				od.list = append(od.list, Entry{Index: uint16(index), ObjectType: uint8(object_type), odObject: variable, extension: nil})

			case OBJ_ARR:

			}

			od.list = append(od.list, Entry{Index: uint16(index), ObjectType: uint8(object_type), odObject: nil, extension: nil})
			fmt.Println(section.Name())
		}
		// for _, key := range section.Keys() {
		// 	fmt.Println(key.Name())
		// }

	}

	return ObjectDictionary{}

}
